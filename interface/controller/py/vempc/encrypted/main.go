package main

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"

	"github.com/tuneinsight/lattigo/v6/core/rlwe"
	"github.com/tuneinsight/lattigo/v6/schemes/ckks"
	"gonum.org/v1/gonum/mat"

	"qqs3c_vempc_encrypted/core"
	"qqs3c_vempc_encrypted/solvers"
)

type engineConfig struct {
	Backend     string      `json:"backend"`
	DT          float64     `json:"dt"`
	N           int         `json:"N"`
	A           [][]float64 `json:"A"`
	B           [][]float64 `json:"B"`
	C           [][]float64 `json:"C"`
	L           [][]float64 `json:"L"`
	QDiag       []float64   `json:"QDiag"`
	RDiag       []float64   `json:"RDiag"`
	QfScale     float64     `json:"QfScale"`
	AlphaMax    float64     `json:"alphaMax"`
	UMax        float64     `json:"uMax"`
	Sigma0      float64     `json:"sigma0"`
	LambdaParam float64     `json:"lambda"`
	K           int         `json:"K"`
	ChebOrder   int         `json:"chebOrder"`
	ChebBound   float64     `json:"chebBound"`
	ChebEta     float64     `json:"chebEta"`
	ChebClip    bool        `json:"chebClip"`
	X0          []float64   `json:"x0"`

	EncryptedCacheSteps int   `json:"encryptedCacheSteps"`
	EncryptedWorkers    int   `json:"encryptedWorkers"`
	CKKSLogN            int   `json:"ckksLogN"`
	CKKSLogQ            []int `json:"ckksLogQ"`
	CKKSLogP            []int `json:"ckksLogP"`
	CKKSLogDefaultScale int   `json:"ckksLogDefaultScale"`
}

type request struct {
	Type string    `json:"type"`
	Y    []float64 `json:"y,omitempty"`
}

type response struct {
	U           float64   `json:"u,omitempty"`
	WSum        float64   `json:"w_sum,omitempty"`
	AcceptNum   int       `json:"accept_num,omitempty"`
	XHat        []float64 `json:"x_hat,omitempty"`
	EncryptedMS float64   `json:"encrypted_ms,omitempty"`
	Error       string    `json:"error,omitempty"`
}

type observer struct {
	A          *mat.Dense
	B          *mat.Dense
	C          *mat.Dense
	L          *mat.Dense
	x          []float64
	cx         []float64
	innovation []float64
	linj       []float64
	ax         []float64
	bu         []float64
	stateOut   []float64
}

func newObserver(A, B, C, L *mat.Dense, x0 []float64) *observer {
	x := make([]float64, len(x0))
	copy(x, x0)
	yDim, _ := C.Dims()
	n := len(x0)
	return &observer{
		A:          A,
		B:          B,
		C:          C,
		L:          L,
		x:          x,
		cx:         make([]float64, yDim),
		innovation: make([]float64, yDim),
		linj:       make([]float64, n),
		ax:         make([]float64, n),
		bu:         make([]float64, n),
		stateOut:   make([]float64, n),
	}
}

func (o *observer) Correct(y []float64) []float64 {
	core.MatVecMulInto(o.cx, o.C, o.x)
	for i := range y {
		o.innovation[i] = y[i] - o.cx[i]
	}
	core.MatVecMulInto(o.linj, o.L, o.innovation)
	for i := range o.x {
		o.x[i] += o.linj[i]
	}
	copy(o.stateOut, o.x)
	return o.stateOut
}

func (o *observer) Predict(u []float64) {
	core.MatVecMulInto(o.ax, o.A, o.x)
	core.MatVecMulInto(o.bu, o.B, u)
	for i := range o.x {
		o.x[i] = o.ax[i] + o.bu[i]
	}
}

type encryptedController struct {
	workers     []*onlineWorker
	muEnc       *inputEncryptor
	bEnc        *inputEncryptor
	variational *core.VariationalMPC
	penalty     *core.ConstraintPenalty
	dim         int
	p           int
	kChunk      int
	nWorkers    int
	nm          int
	mIn         int
	chebEta     float64
	logW        []float64
	uHat        []float64
	u0          []float64
	cacheLen    int
	stepCount   int
}

type controlDetail struct {
	U         []float64
	WSum      float64
	AcceptNum int
	MS        float64
}

func (c *encryptedController) Step(x []float64) controlDetail {
	start := time.Now()
	tc := c.stepCount % c.cacheLen
	c.stepCount++

	mU := computeMU(c.variational, x)
	b := computeB(c.penalty, mU, x)
	ctMU := c.muEnc.EncryptTiled(mU, c.dim, c.kChunk, c.workers[0].cache.Load("ct_Lu", tc).Level())
	ctB := c.bEnc.EncryptTiled(b, c.p, c.kChunk, c.workers[0].cache.Load("ct_Gamma", tc).Level())

	var wg sync.WaitGroup
	errCh := make(chan error, c.nWorkers)
	for _, worker := range c.workers {
		worker := worker
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := worker.runCycle(tc, ctMU, ctB); err != nil {
				errCh <- err
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			panic(err)
		}
	}

	for j := 0; j < c.nm; j++ {
		c.uHat[j] = 0
	}

	logWMax := math.Inf(-1)
	acceptNum := 0
	for w, worker := range c.workers {
		base := w * c.kChunk
		for i := 0; i < c.kChunk; i++ {
			if worker.sVals[i] <= 1e-6 {
				acceptNum++
			}
			c.logW[base+i] = -c.chebEta * worker.sVals[i]
			if c.logW[base+i] > logWMax {
				logWMax = c.logW[base+i]
			}
		}
	}
	if math.IsInf(logWMax, -1) {
		logWMax = 0
	}

	var wSum float64
	for w, worker := range c.workers {
		base := w * c.kChunk
		for i := 0; i < c.kChunk; i++ {
			wi := math.Exp(c.logW[base+i] - logWMax)
			wSum += wi
			off := i * c.dim
			for j := 0; j < c.nm; j++ {
				c.uHat[j] += wi * worker.uFlat[off+j]
			}
		}
	}
	if wSum > 0 {
		for j := 0; j < c.nm; j++ {
			c.uHat[j] /= wSum
		}
	}
	copy(c.u0, c.uHat[:c.mIn])
	return controlDetail{
		U:         c.u0,
		WSum:      wSum,
		AcceptNum: acceptNum,
		MS:        time.Since(start).Seconds() * 1000.0,
	}
}

type onlineController struct {
	observer  *observer
	encrypted *encryptedController
}

func (c *onlineController) Step(y []float64) response {
	x := c.observer.Correct(y)
	detail := c.encrypted.Step(x)
	c.observer.Predict(detail.U)
	return response{
		U:           detail.U[0],
		WSum:        detail.WSum,
		AcceptNum:   detail.AcceptNum,
		XHat:        x,
		EncryptedMS: detail.MS,
	}
}

type cacheMeta struct {
	ConfigHash          string `json:"configHash"`
	Dim                 int    `json:"dim"`
	P                   int    `json:"p"`
	K                   int    `json:"K"`
	NWorkers            int    `json:"nWorkers"`
	CacheLen            int    `json:"cacheLen"`
	LogN                int    `json:"logN"`
	LogQ                []int  `json:"logQ"`
	LogP                []int  `json:"logP"`
	LogDefaultScale     int    `json:"logDefaultScale"`
	ChebOrder           int    `json:"chebOrder"`
	StackedInputHorizon int    `json:"stackedInputHorizon"`
}

func newOnlineController(cfg engineConfig, rawConfig []byte, allowCacheGenerate bool) (*onlineController, error) {
	if cfg.Backend != "" && cfg.Backend != "variational" {
		return nil, fmt.Errorf("encrypted engine supports only variational backend, got %q", cfg.Backend)
	}

	A, err := denseFrom2D(cfg.A)
	if err != nil {
		return nil, err
	}
	B, err := denseFrom2D(cfg.B)
	if err != nil {
		return nil, err
	}
	C, err := denseFrom2D(cfg.C)
	if err != nil {
		return nil, err
	}
	L, err := denseFrom2D(cfg.L)
	if err != nil {
		return nil, err
	}

	n, _ := A.Dims()
	_, mIn := B.Dims()
	if cfg.N <= 0 || cfg.K <= 0 || cfg.Sigma0 <= 0 || cfg.LambdaParam <= 0 {
		return nil, fmt.Errorf("invalid encrypted VEMPC configuration")
	}
	if len(cfg.QDiag) != n || len(cfg.RDiag) != mIn {
		return nil, fmt.Errorf("QDiag/RDiag dimensions do not match A/B")
	}
	if len(cfg.X0) == 0 {
		cfg.X0 = make([]float64, n)
	}
	if len(cfg.X0) != n {
		return nil, fmt.Errorf("x0 length %d does not match state dimension %d", len(cfg.X0), n)
	}

	Q := diagDense(cfg.QDiag)
	R := diagDense(cfg.RDiag)
	Qf := mat.NewDense(n, n, nil)
	Qf.Scale(cfg.QfScale, Q)

	Gx := mat.NewDense(2, n, nil)
	Gx.Set(0, 1, 1.0)
	Gx.Set(1, 1, -1.0)
	hx := []float64{cfg.AlphaMax, cfg.AlphaMax}
	Gu := stackIdentity(mIn)
	hu := []float64{cfg.UMax, cfg.UMax}

	mpc := core.NewMPCProblem(A, B, Q, R, Qf, cfg.N)
	G, hOfX0 := mpc.BuildConstraintMatrices(Gx, hx, Gu, hu)
	penalty := core.NewConstraintPenalty(G, hOfX0, "indicator")

	Sigma0 := scaledIdentity(mpc.StackedInputDim(), cfg.Sigma0*cfg.Sigma0)
	variational := core.NewVariationalMPC(mpc, penalty, cfg.LambdaParam, Sigma0)

	LU := variational.LU
	var gamma mat.Dense
	gamma.Mul(penalty.G, LU)
	dim, _ := LU.Dims()
	p, _ := gamma.Dims()

	nWorkers := normalizeWorkerCount(cfg.K, cfg.EncryptedWorkers)
	kChunk := cfg.K / nWorkers
	cacheLen := cfg.EncryptedCacheSteps
	if cacheLen <= 0 {
		cacheLen = 1
	}

	paramsLit := ckks.ParametersLiteral{
		LogN:            defaultInt(cfg.CKKSLogN, 15),
		LogQ:            defaultInts(cfg.CKKSLogQ, []int{50, 45, 45, 45, 45, 45, 45, 45, 45}),
		LogP:            defaultInts(cfg.CKKSLogP, []int{50}),
		LogDefaultScale: defaultInt(cfg.CKKSLogDefaultScale, 40),
	}
	params, err := ckks.NewParametersFromLiteral(paramsLit)
	if err != nil {
		return nil, err
	}
	slots := params.MaxSlots()
	slotWidth := maxInt(dim, p)
	if slots < slotWidth*kChunk {
		return nil, fmt.Errorf("CKKS slots %d too small for slotWidth=%d, KChunk=%d", slots, slotWidth, kChunk)
	}

	cacheDir := filepath.Join("runtime", "ckks_cache")
	configHash := hashBytes(rawConfig)
	meta := cacheMeta{
		ConfigHash:          configHash,
		Dim:                 dim,
		P:                   p,
		K:                   cfg.K,
		NWorkers:            nWorkers,
		CacheLen:            cacheLen,
		LogN:                paramsLit.LogN,
		LogQ:                paramsLit.LogQ,
		LogP:                paramsLit.LogP,
		LogDefaultScale:     paramsLit.LogDefaultScale,
		ChebOrder:           cfg.ChebOrder,
		StackedInputHorizon: mpc.StackedInputDim(),
	}
	if err := ensureOfflineCache(cacheDir, meta, params, LU, &gamma, dim, p, cfg.K, nWorkers, cacheLen, allowCacheGenerate); err != nil {
		return nil, err
	}

	cryptoDir := filepath.Join(cacheDir, "crypto")
	params, sk, pk, rlk := loadCrypto(cryptoDir)
	rotations := collectOnlineRotations(kChunk, dim, p, params.MaxSlots())
	kgen := ckks.NewKeyGenerator(params)
	gks := make([]*rlwe.GaloisKey, 0, len(rotations))
	for _, rot := range rotations {
		gks = append(gks, kgen.GenGaloisKeyNew(params.GaloisElementForRotation(rot), sk))
	}
	evalKeys := rlwe.NewMemEvaluationKeySet(rlk, gks...)

	chebCoeffs := solvers.ChebyshevReLUCoeffs(cfg.ChebOrder, cfg.ChebBound, 0)
	polyZ := scalePoly(chebToPower(chebCoeffs), cfg.ChebBound)
	tauS := float64(p) * evalPolyReal(polyZ, 0.0)

	workers := make([]*onlineWorker, nWorkers)
	for i := 0; i < nWorkers; i++ {
		workers[i] = newOnlineWorker(
			workerCacheDir(cacheDir, i, nWorkers),
			kChunk,
			dim,
			p,
			params.MaxSlots(),
			params,
			sk,
			evalKeys,
			polyZ,
			tauS,
		)
	}

	return &onlineController{
		observer: newObserver(A, B, C, L, cfg.X0),
		encrypted: &encryptedController{
			workers:     workers,
			muEnc:       newInputEncryptor(params, pk, params.MaxSlots()),
			bEnc:        newInputEncryptor(params, pk, params.MaxSlots()),
			variational: variational,
			penalty:     penalty,
			dim:         dim,
			p:           p,
			kChunk:      kChunk,
			nWorkers:    nWorkers,
			nm:          mpc.StackedInputDim(),
			mIn:         mIn,
			chebEta:     cfg.ChebEta,
			logW:        make([]float64, cfg.K),
			uHat:        make([]float64, mpc.StackedInputDim()),
			u0:          make([]float64, mIn),
			cacheLen:    cacheLen,
		},
	}, nil
}

func ensureOfflineCache(cacheDir string, meta cacheMeta, params ckks.Parameters, LU *mat.Dense, gamma *mat.Dense, dim int, p int, K int, nWorkers int, cacheLen int, allowGenerate bool) error {
	metaPath := filepath.Join(cacheDir, "cache_meta.json")
	if cacheValid(metaPath, meta, cacheDir, nWorkers) {
		fmt.Fprintln(os.Stderr, "encrypted offline cache already valid")
		return nil
	}
	if !allowGenerate {
		return fmt.Errorf("encrypted offline cache is missing or stale; run offline_vempc.py before starting ctrl_vempc.py")
	}
	fmt.Fprintf(os.Stderr, "generating encrypted offline cache: K=%d, workers=%d, cache_steps=%d\n", K, nWorkers, cacheLen)
	if err := os.RemoveAll(cacheDir); err != nil {
		return err
	}
	cryptoDir := filepath.Join(cacheDir, "crypto")
	if err := os.MkdirAll(cryptoDir, 0o755); err != nil {
		return err
	}

	kgen := ckks.NewKeyGenerator(params)
	sk, pk := kgen.GenKeyPairNew()
	rlk := kgen.GenRelinearizationKeyNew(sk)
	saveCryptoMaterial(cryptoDir, params, sk, pk, rlk)
	fmt.Fprintln(os.Stderr, "generated CKKS parameters and key material")

	slots := params.MaxSlots()
	kChunk := K / nWorkers
	rotations := collectPackRotations(kChunk, dim, p, slots)
	gks := make([]*rlwe.GaloisKey, 0, len(rotations))
	for _, rot := range rotations {
		gks = append(gks, kgen.GenGaloisKeyNew(params.GaloisElementForRotation(rot), sk))
	}
	evalKeys := rlwe.NewMemEvaluationKeySet(rlk, gks...)
	encoder := ckks.NewEncoder(params)
	encryptor := ckks.NewEncryptor(params, pk)
	encLU := encryptCyclicDiagonalsSquare(LU, dim, slots, encoder, encryptor, params)
	encGamma := encryptCyclicDiagonalsGamma(gamma, p, dim, slots, encoder, encryptor, params)
	fmt.Fprintln(os.Stderr, "encrypted LU and Gamma diagonals")

	var wg sync.WaitGroup
	var progressMu sync.Mutex
	completed := 0
	bar := newProgressBar("ckks_offline", nWorkers*cacheLen)
	errCh := make(chan error, nWorkers)
	for workerID := 0; workerID < nWorkers; workerID++ {
		workerID := workerID
		wg.Add(1)
		go func() {
			defer wg.Done()
			workerDir := workerCacheDir(cacheDir, workerID, nWorkers)
			if err := os.MkdirAll(workerDir, 0o755); err != nil {
				errCh <- err
				return
			}
			workerEncoder := ckks.NewEncoder(params)
			workerEvaluator := ckks.NewEvaluator(params, evalKeys)
			maskLU := makeMaskPlaintext(dim, slots, params, workerEncoder)
			maskG := makeMaskPlaintext(p, slots, params, workerEncoder)
			scratchLU := newEncMatVecScratch(dim, slots, params)
			scratchG := newEncMatVecScratch(p, slots, params)
			rng := newDeterministicNormal(workerID + 1)

			ctLUList := make([]*rlwe.Ciphertext, kChunk)
			ctGList := make([]*rlwe.Ciphertext, kChunk)
			xi := make([]float64, dim)
			xiPad := make([]float64, p)
			for t := 0; t < cacheLen; t++ {
				for s := 0; s < kChunk; s++ {
					for i := 0; i < dim; i++ {
						xi[i] = rng.NormFloat64()
					}
					for i := range xiPad {
						xiPad[i] = 0
					}
					copy(xiPad, xi)
					ctLUList[s] = encMatVec(encLU, xi, dim, workerEncoder, workerEvaluator, scratchLU)
					ctGList[s] = encMatVec(encGamma, xiPad, p, workerEncoder, workerEvaluator, scratchG)
				}
				if err := writeCiphertext(workerCachePath(workerDir, "ct_Lu", t), packCiphertexts(ctLUList, dim, slots, maskLU, workerEvaluator)); err != nil {
					errCh <- err
					return
				}
				if err := writeCiphertext(workerCachePath(workerDir, "ct_Gamma", t), packCiphertexts(ctGList, p, slots, maskG, workerEvaluator)); err != nil {
					errCh <- err
					return
				}
				progressMu.Lock()
				completed++
				bar.Update(completed)
				progressMu.Unlock()
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(metaPath, data, 0o644)
}

func cacheValid(metaPath string, want cacheMeta, cacheDir string, nWorkers int) bool {
	data, err := os.ReadFile(metaPath)
	if err != nil {
		return false
	}
	var got cacheMeta
	if err := json.Unmarshal(data, &got); err != nil {
		return false
	}
	if !reflect.DeepEqual(got, want) {
		return false
	}
	for i := 0; i < nWorkers; i++ {
		workerDir := workerCacheDir(cacheDir, i, nWorkers)
		if _, err := os.Stat(workerCachePath(workerDir, "ct_Lu", 0)); err != nil {
			return false
		}
		if _, err := os.Stat(workerCachePath(workerDir, "ct_Gamma", 0)); err != nil {
			return false
		}
	}
	return true
}

func loadConfig(path string) (engineConfig, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return engineConfig{}, nil, err
	}
	var cfg engineConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return engineConfig{}, nil, err
	}
	return cfg, data, nil
}

func denseFrom2D(src [][]float64) (*mat.Dense, error) {
	if len(src) == 0 || len(src[0]) == 0 {
		return nil, fmt.Errorf("matrix cannot be empty")
	}
	rows := len(src)
	cols := len(src[0])
	data := make([]float64, 0, rows*cols)
	for i := 0; i < rows; i++ {
		if len(src[i]) != cols {
			return nil, fmt.Errorf("ragged matrix row %d", i)
		}
		data = append(data, src[i]...)
	}
	return mat.NewDense(rows, cols, data), nil
}

func normalizeWorkerCount(K int, requested int) int {
	if requested <= 0 {
		requested = 2
	}
	if requested > K {
		requested = K
	}
	for requested > 1 && K%requested != 0 {
		requested--
	}
	return requested
}

func defaultInt(value int, fallback int) int {
	if value == 0 {
		return fallback
	}
	return value
}

func defaultInts(value []int, fallback []int) []int {
	if len(value) == 0 {
		return fallback
	}
	return value
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

type deterministicNormal struct {
	state uint64
}

func newDeterministicNormal(seed int) *deterministicNormal {
	if seed <= 0 {
		seed = 1
	}
	return &deterministicNormal{state: uint64(seed)}
}

func (r *deterministicNormal) Float64() float64 {
	r.state = r.state*6364136223846793005 + 1442695040888963407
	v := r.state >> 11
	return float64(v) * (1.0 / (1 << 53))
}

func (r *deterministicNormal) NormFloat64() float64 {
	u1 := r.Float64()
	if u1 <= 0 {
		u1 = math.SmallestNonzeroFloat64
	}
	u2 := r.Float64()
	return math.Sqrt(-2*math.Log(u1)) * math.Cos(2*math.Pi*u2)
}

func main() {
	configPath := flag.String("config", "", "path to controller config JSON")
	offlineOnly := flag.Bool("offline-only", false, "generate encrypted offline cache and exit")
	flag.Parse()
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing --config path")
		os.Exit(1)
	}

	cfg, rawConfig, err := loadConfig(*configPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	controller, err := newOnlineController(cfg, rawConfig, *offlineOnly)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if *offlineOnly {
		return
	}

	decoder := json.NewDecoder(bufio.NewReader(os.Stdin))
	encoder := json.NewEncoder(os.Stdout)
	for {
		var req request
		if err := decoder.Decode(&req); err != nil {
			break
		}
		switch req.Type {
		case "shutdown":
			return
		case "measure":
			if len(req.Y) != 2 {
				_ = encoder.Encode(response{Error: "measurement vector must have length 2"})
				continue
			}
			resp := controller.Step(req.Y)
			if err := encoder.Encode(resp); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		default:
			_ = encoder.Encode(response{Error: "unsupported request type"})
		}
	}
}

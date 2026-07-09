package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand"
	"os"

	"gonum.org/v1/gonum/mat"

	"qqs3c_vempc/core"
	"qqs3c_vempc/internal/engineconfig"
	"qqs3c_vempc/solvers"
)

type request struct {
	Type string    `json:"type"`
	Y    []float64 `json:"y,omitempty"`
}

type response struct {
	U         float64   `json:"u,omitempty"`
	WSum      float64   `json:"w_sum,omitempty"`
	AcceptNum int       `json:"accept_num,omitempty"`
	XHat      []float64 `json:"x_hat,omitempty"`
	Error     string    `json:"error,omitempty"`
}

type observer struct {
	// The observer lives inside the Go engine so the Python side can stay
	// stateless and send only measured outputs.
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
	// Luenberger-style correction step: x <- x + L(y - Cx).
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
	// One-step prediction used to carry the estimated state to the next sample.
	core.MatVecMulInto(o.ax, o.A, o.x)
	core.MatVecMulInto(o.bu, o.B, u)
	for i := range o.x {
		o.x[i] = o.ax[i] + o.bu[i]
	}
}

type onlineController struct {
	// This struct holds the long-lived controller state that would be expensive
	// to rebuild for every sample: observer state, condensed matrices, warm
	// starts, and variational sampling settings.
	backend      string
	observer     *observer
	stdSolver    *solvers.StandardQPSolver
	variational  *core.VariationalMPC
	penalty      *core.ConstraintPenalty
	S            *mat.Dense
	hOfX0        func([]float64) []float64
	mIn          int
	warm         []float64
	chebCoeffs   []float64
	chebBound    float64
	chebEta      float64
	chebClip     bool
	kSamples     int
	rng          *rand.Rand
	varWorkspace *core.VariationalWorkspace
}

func newOnlineController(cfg engineconfig.Config) (*onlineController, error) {
	// The Python side exports raw matrices and tuning values. This constructor
	// validates them and builds the condensed MPC/variational structures once.
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
	if len(cfg.QDiag) != n {
		return nil, fmt.Errorf("QDiag length %d does not match state dimension %d", len(cfg.QDiag), n)
	}
	if len(cfg.RDiag) != mIn {
		return nil, fmt.Errorf("RDiag length %d does not match input dimension %d", len(cfg.RDiag), mIn)
	}
	if cfg.N <= 0 {
		return nil, fmt.Errorf("N must be positive")
	}
	if cfg.QfScale <= 0 || cfg.Sigma0 <= 0 || cfg.LambdaParam <= 0 || cfg.K <= 0 {
		return nil, fmt.Errorf("invalid MPC parameters in config")
	}
	if cfg.AlphaMax <= 0 || cfg.UMax <= 0 {
		return nil, fmt.Errorf("invalid alphaMax or uMax in config")
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

	// Match the QQS3C balancing region: constrain only alpha and the scalar
	// command magnitude in the online controller.
	Gx := mat.NewDense(2, n, nil)
	Gx.Set(0, 1, 1.0)
	Gx.Set(1, 1, -1.0)
	hx := []float64{cfg.AlphaMax, cfg.AlphaMax}
	Gu := stackIdentity(mIn)
	hu := []float64{cfg.UMax, cfg.UMax}

	mpc := core.NewMPCProblem(A, B, Q, R, Qf, cfg.N)
	G, hOfX0 := mpc.BuildConstraintMatrices(Gx, hx, Gu, hu)
	penalty := core.NewConstraintPenalty(G, hOfX0, "indicator")

	engine := &onlineController{
		backend:   cfg.Backend,
		observer:  newObserver(A, B, C, L, cfg.X0),
		S:         mpc.S,
		hOfX0:     hOfX0,
		mIn:       mIn,
		penalty:   penalty,
		chebBound: cfg.ChebBound,
		chebEta:   cfg.ChebEta,
		chebClip:  cfg.ChebClip,
		kSamples:  cfg.K,
		rng:       rand.New(rand.NewSource(0)),
	}

	if cfg.Backend == "" || cfg.Backend == "variational" {
		// Sigma0 is diagonal here because the Python config exports a scalar prior
		// width. The variational object then derives SigmaU and its Cholesky factor.
		sigma0 := scaledIdentity(mpc.StackedInputDim(), cfg.Sigma0*cfg.Sigma0)
		engine.variational = core.NewVariationalMPC(mpc, penalty, cfg.LambdaParam, sigma0)
		engine.chebCoeffs = solvers.ChebyshevReLUCoeffs(cfg.ChebOrder, cfg.ChebBound, 0)
		engine.varWorkspace = core.NewVariationalWorkspace(engine.variational, cfg.K)
		engine.backend = "variational"
		return engine, nil
	}

	if cfg.Backend == "standard" {
		engine.stdSolver = solvers.NewStandardQPSolver(mpc.H, G)
		return engine, nil
	}

	return nil, fmt.Errorf("unsupported backend %q", cfg.Backend)
}

func (c *onlineController) Step(y []float64) response {
	// Each online cycle is "correct on y -> solve MPC -> predict with applied u".
	x := c.observer.Correct(y)
	switch c.backend {
	case "standard":
		u, useq, _ := solvers.SolveStandardMPC(x, c.S, c.hOfX0, c.mIn, c.stdSolver, c.warm)
		if useq != nil {
			c.warm = append(c.warm[:0], useq...)
		}
		c.observer.Predict(u)
		return response{U: u[0], XHat: x}
	default:
		// The variational controller samples candidate input sequences, scores
		// their constraint violations, and returns the weighted first action.
		seed := c.rng.Int63()
		u, _, wSum, acceptNum := solvers.SampleVariationalControlWithWorkspace(
			x,
			c.variational,
			c.penalty,
			c.chebCoeffs,
			c.chebBound,
			c.chebEta,
			c.chebClip,
			seed,
			c.varWorkspace,
		)
		c.observer.Predict(u)
		return response{U: u[0], WSum: wSum, AcceptNum: acceptNum, XHat: x}
	}
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

func diagDense(vals []float64) *mat.Dense {
	n := len(vals)
	out := mat.NewDense(n, n, nil)
	for i := 0; i < n; i++ {
		out.Set(i, i, vals[i])
	}
	return out
}

func stackIdentity(n int) *mat.Dense {
	out := mat.NewDense(2*n, n, nil)
	for i := 0; i < n; i++ {
		out.Set(i, i, 1.0)
		out.Set(n+i, i, -1.0)
	}
	return out
}

func scaledIdentity(n int, scale float64) *mat.Dense {
	out := mat.NewDense(n, n, nil)
	for i := 0; i < n; i++ {
		out.Set(i, i, scale)
	}
	return out
}

func main() {
	configPath := flag.String("config", "", "path to controller config JSON")
	flag.Parse()
	if *configPath == "" {
		fmt.Fprintln(os.Stderr, "missing --config path")
		os.Exit(1)
	}

	cfg, ok := engineconfig.Load(*configPath)
	if !ok {
		fmt.Fprintf(os.Stderr, "failed to load config %s\n", *configPath)
		os.Exit(1)
	}
	controller, err := newOnlineController(cfg)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
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
			// The stdin/stdout protocol is intentionally tiny so Python can act as
			// a simple bridge between the QQS3C plant socket and this Go engine.
			if len(req.Y) != 2 {
				_ = encoder.Encode(response{Error: "measurement vector must have length 2"})
				continue
			}
			if err := encoder.Encode(controller.Step(req.Y)); err != nil {
				fmt.Fprintln(os.Stderr, err)
				os.Exit(1)
			}
		default:
			_ = encoder.Encode(response{Error: "unsupported request type"})
		}
	}
}

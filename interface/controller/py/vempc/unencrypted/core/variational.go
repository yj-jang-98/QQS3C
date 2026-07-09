package core

import (
	"math"
	"math/rand"

	"gonum.org/v1/gonum/mat"
)

type VariationalMPC struct {
	// SigmaU and LU are the tilted covariance and its Cholesky factor used to
	// sample candidate control sequences efficiently at runtime.
	MPC         *MPCProblem
	Penalty     *ConstraintPenalty
	LambdaParam float64
	Sigma0      *mat.Dense
	SigmaU      *mat.Dense
	LU          *mat.Dense
}

type VariationalWorkspace struct {
	Mean      []float64
	MeanTmp   []float64
	Xi        *mat.Dense
	Noise     *mat.Dense
	Samples   *mat.Dense
	Residuals *mat.Dense
	Surrogate *mat.Dense
	LogW      []float64
	Weights   []float64
	UHat      []float64
	U0        []float64
	Feasible  []bool
	Sampler   *rand.Rand
}

func NewVariationalMPC(mpc *MPCProblem, penalty *ConstraintPenalty, lambdaParam float64, sigma0 *mat.Dense) *VariationalMPC {
	SigmaU := computeSigmaU(sigma0, mpc.H, lambdaParam)
	LU := choleskyLower(SigmaU)
	return &VariationalMPC{
		MPC:         mpc,
		Penalty:     penalty,
		LambdaParam: lambdaParam,
		Sigma0:      sigma0,
		SigmaU:      SigmaU,
		LU:          LU,
	}
}

func NewVariationalWorkspace(v *VariationalMPC, K int) *VariationalWorkspace {
	Nm := v.MPC.Nm
	p := v.Penalty.P
	mIn := v.MPC.InputDim()
	return &VariationalWorkspace{
		Mean:      make([]float64, Nm),
		MeanTmp:   make([]float64, Nm),
		Xi:        mat.NewDense(K, Nm, nil),
		Noise:     mat.NewDense(K, Nm, nil),
		Samples:   mat.NewDense(K, Nm, nil),
		Residuals: mat.NewDense(K, p, nil),
		Surrogate: mat.NewDense(K, p, nil),
		LogW:      make([]float64, K),
		Weights:   make([]float64, K),
		UHat:      make([]float64, Nm),
		U0:        make([]float64, mIn),
		Feasible:  make([]bool, K),
		Sampler:   rand.New(rand.NewSource(0)),
	}
}

func computeSigmaU(Sigma0, H *mat.Dense, lambda float64) *mat.Dense {
	// SigmaU = (Sigma0^{-1} + H/lambda)^{-1}.
	var Sigma0Inv mat.Dense
	_ = Sigma0Inv.Inverse(Sigma0)
	var scaledH mat.Dense
	scaledH.Scale(1.0/lambda, H)
	var tmp mat.Dense
	tmp.Add(&Sigma0Inv, &scaledH)
	var SigmaU mat.Dense
	_ = SigmaU.Inverse(&tmp)
	return &SigmaU
}

func choleskyLower(A *mat.Dense) *mat.Dense {
	n, _ := A.Dims()
	sym := mat.NewSymDense(n, nil)
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sym.SetSym(i, j, A.At(i, j))
		}
	}
	var chol mat.Cholesky
	_ = chol.Factorize(sym)
	var L mat.TriDense
	chol.LTo(&L)
	rows, cols := L.Dims()
	out := mat.NewDense(rows, cols, nil)
	out.Copy(&L)
	return out
}

func (v *VariationalMPC) mU(x0 []float64) []float64 {
	// Mean of the tilted Gaussian over stacked control sequences.
	Stx := MatVecMul(v.MPC.S.T(), x0)
	SigmaStx := MatVecMul(v.SigmaU, Stx)
	return VecScale(SigmaStx, -(1.0 / v.LambdaParam))
}

func (v *VariationalMPC) mUInto(dst []float64, tmp []float64, x0 []float64) {
	MatVecMulInto(tmp, v.MPC.S.T(), x0)
	MatVecMulInto(dst, v.SigmaU, tmp)
	scale := -(1.0 / v.LambdaParam)
	for i := range dst {
		dst[i] *= scale
	}
}

func (v *VariationalMPC) SampleKappaTilde(x0 []float64, K int, seed int64) *mat.Dense {
	// Draw K stacked input sequences U^(i) ~ N(mU(x0), SigmaU).
	rng := rand.New(rand.NewSource(seed))
	Nm := v.MPC.Nm
	mean := v.mU(x0)
	xi := mat.NewDense(K, Nm, nil)
	for i := 0; i < K; i++ {
		for j := 0; j < Nm; j++ {
			xi.Set(i, j, rng.NormFloat64())
		}
	}
	var noise mat.Dense
	noise.Mul(xi, v.LU.T())
	samples := mat.NewDense(K, Nm, nil)
	for i := 0; i < K; i++ {
		for j := 0; j < Nm; j++ {
			samples.Set(i, j, mean[j]+noise.At(i, j))
		}
	}
	return samples
}

func (v *VariationalMPC) SampleKappaTildeInto(ws *VariationalWorkspace, x0 []float64, seed int64) *mat.Dense {
	ws.Sampler.Seed(seed)
	v.mUInto(ws.Mean, ws.MeanTmp, x0)

	xiData := ws.Xi.RawMatrix().Data
	for i := range xiData {
		xiData[i] = ws.Sampler.NormFloat64()
	}

	ws.Noise.Mul(ws.Xi, v.LU.T())

	noiseRaw := ws.Noise.RawMatrix()
	sampleRaw := ws.Samples.RawMatrix()
	Nm := len(ws.Mean)
	for i := 0; i < noiseRaw.Rows; i++ {
		noiseRow := noiseRaw.Data[i*noiseRaw.Stride : i*noiseRaw.Stride+Nm]
		sampleRow := sampleRaw.Data[i*sampleRaw.Stride : i*sampleRaw.Stride+Nm]
		for j := 0; j < Nm; j++ {
			sampleRow[j] = ws.Mean[j] + noiseRow[j]
		}
	}
	return ws.Samples
}

type WeightOptions struct {
	ChebCoeffs []float64
	ChebBound  float64
	ChebClip   bool
	ChebEta    float64
	Eps        float64
}

func (v *VariationalMPC) ComputeWeights(U *mat.Dense, x0 []float64, opts WeightOptions) ([]float64, float64) {
	K, _ := U.Dims()
	if opts.Eps == 0 {
		opts.Eps = 1e-12
	}

	logWeights := make([]float64, K)
	for i := range logWeights {
		logWeights[i] = math.Inf(-1)
	}

	// Residuals g(U; x0) are run through a Chebyshev ReLU surrogate so the same
	// structure can later match the encrypted VEMPC path.
	residuals := v.Penalty.ConstraintResidualMat(U, x0)
	h := chebReLU(residuals, opts.ChebCoeffs, opts.ChebBound, opts.ChebClip)
	threshold := chebVal(0.0, opts.ChebCoeffs)

	eta := opts.ChebEta
	if eta == 0 {
		eta = 1.0
	}
	for i := 0; i < K; i++ {
		// Only positive residual mass above the surrogate's zero-level threshold
		// contributes to the exponential penalty.
		var s float64
		_, p := h.Dims()
		for j := 0; j < p; j++ {
			val := h.At(i, j)
			if val > threshold {
				s += val
			}
		}
		logWeights[i] = -eta * s
	}

	maxLog := math.Inf(-1)
	for _, v := range logWeights {
		if math.IsInf(v, -1) {
			continue
		}
		if v > maxLog {
			maxLog = v
		}
	}
	if math.IsInf(maxLog, -1) {
		maxLog = 0.0
	}

	weights := make([]float64, K)
	// Normalize in log-space for numerical stability when eta is large.
	var sum float64
	for i, v := range logWeights {
		if math.IsInf(v, -1) {
			weights[i] = 0
			continue
		}
		w := math.Exp(v - maxLog)
		weights[i] = w
		sum += w
	}
	if sum > 0 {
		for i := range weights {
			weights[i] /= sum
		}
	}
	return weights, sum
}

func (v *VariationalMPC) ComputeWeightsFromResidualsInto(
	residuals *mat.Dense,
	opts WeightOptions,
	surrogate *mat.Dense,
	logWeights []float64,
	weights []float64,
) float64 {
	K, _ := residuals.Dims()
	if opts.Eps == 0 {
		opts.Eps = 1e-12
	}

	for i := 0; i < K; i++ {
		logWeights[i] = math.Inf(-1)
	}

	chebReLUInto(surrogate, residuals, opts.ChebCoeffs, opts.ChebBound, opts.ChebClip)
	threshold := chebVal(0.0, opts.ChebCoeffs)

	eta := opts.ChebEta
	if eta == 0 {
		eta = 1.0
	}

	raw := surrogate.RawMatrix()
	for i := 0; i < raw.Rows; i++ {
		row := raw.Data[i*raw.Stride : i*raw.Stride+raw.Cols]
		var s float64
		for _, val := range row {
			if val > threshold {
				s += val
			}
		}
		logWeights[i] = -eta * s
	}

	maxLog := math.Inf(-1)
	for i := 0; i < K; i++ {
		v := logWeights[i]
		if math.IsInf(v, -1) {
			continue
		}
		if v > maxLog {
			maxLog = v
		}
	}
	if math.IsInf(maxLog, -1) {
		maxLog = 0.0
	}

	var sum float64
	for i := 0; i < K; i++ {
		v := logWeights[i]
		if math.IsInf(v, -1) {
			weights[i] = 0.0
			continue
		}
		w := math.Exp(v - maxLog)
		weights[i] = w
		sum += w
	}
	if sum > 0 {
		inv := 1.0 / sum
		for i := 0; i < K; i++ {
			weights[i] *= inv
		}
	}
	return sum
}

func chebReLU(residuals *mat.Dense, coeffs []float64, bound float64, clip bool) *mat.Dense {
	r, c := residuals.Dims()
	out := mat.NewDense(r, c, nil)
	chebReLUInto(out, residuals, coeffs, bound, clip)
	return out
}

func chebReLUInto(dst *mat.Dense, residuals *mat.Dense, coeffs []float64, bound float64, clip bool) {
	residualRaw := residuals.RawMatrix()
	outRaw := dst.RawMatrix()
	for i := 0; i < residualRaw.Rows; i++ {
		inRow := residualRaw.Data[i*residualRaw.Stride : i*residualRaw.Stride+residualRaw.Cols]
		outRow := outRaw.Data[i*outRaw.Stride : i*outRaw.Stride+outRaw.Cols]
		for j := 0; j < residualRaw.Cols; j++ {
			t := inRow[j] / bound
			if clip {
				if t < -1.0 {
					t = -1.0
				}
				if t > 1.0 {
					t = 1.0
				}
			}
			y := chebVal(t, coeffs)
			if clip && y < 0.0 {
				y = 0.0
			}
			outRow[j] = y
		}
	}
}

func chebVal(t float64, coeffs []float64) float64 {
	n := len(coeffs)
	if n == 0 {
		return 0
	}
	if n == 1 {
		return coeffs[0]
	}
	b0 := 0.0
	b1 := 0.0
	b2 := 0.0
	for i := n - 1; i >= 1; i-- {
		b2 = b1
		b1 = b0
		b0 = 2*t*b1 - b2 + coeffs[i]
	}
	return t*b0 - b1 + coeffs[0]
}

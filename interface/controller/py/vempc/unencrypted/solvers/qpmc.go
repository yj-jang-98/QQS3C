package solvers

import (
	"gonum.org/v1/gonum/mat"
	"gonum.org/v1/gonum/optimize"
)

type SolverSettings struct {
	MaxIter int
	Ftol    float64
	Gtol    float64
}

type StandardQPSolver struct {
	// This is a soft-constrained condensed MPC solver: violations are penalized
	// quadratically rather than solved as a hard constrained QP.
	H        *mat.Dense
	G        *mat.Dense
	Rho      float64
	Settings SolverSettings
}

func NewStandardQPSolver(H, G *mat.Dense) *StandardQPSolver {
	return &StandardQPSolver{
		H:   H,
		G:   G,
		Rho: 1e4,
		Settings: SolverSettings{
			MaxIter: 500,
			Ftol:    1e-9,
			Gtol:    1e-6,
		},
	}
}

func SolveStandardMPC(
	x0 []float64,
	S *mat.Dense,
	hOfX0 func([]float64) []float64,
	mIn int,
	solver *StandardQPSolver,
	warmStart []float64,
) ([]float64, []float64, error) {
	// q = S^T x0 is the state-dependent linear term in the condensed cost.
	q := matVecMul(S.T(), x0)
	h := hOfX0(x0)

	objective := func(U []float64) float64 {
		cost := 0.5*dot(U, matVecMul(solver.H, U)) + dot(q, U)
		if solver.G == nil {
			return cost
		}
		g := matVecMul(solver.G, U)
		var penalty float64
		for i := range g {
			g[i] -= h[i]
			if g[i] > 0 {
				penalty += g[i] * g[i]
			}
		}
		return cost + solver.Rho*penalty
	}

	gradient := func(grad, U []float64) {
		HU := matVecMul(solver.H, U)
		for i := range grad {
			grad[i] = HU[i] + q[i]
		}
		if solver.G == nil {
			return
		}
		g := matVecMul(solver.G, U)
		v := make([]float64, len(g))
		for i := range g {
			g[i] -= h[i]
			if g[i] > 0 {
				v[i] = 2.0 * g[i]
			}
		}
		gtv := matVecMul(solver.G.T(), v)
		for i := range grad {
			grad[i] += solver.Rho * gtv[i]
		}
	}

	var xInit []float64
	if warmStart != nil {
		// Reuse the previous solution when the state evolves slowly.
		xInit = append([]float64(nil), warmStart...)
	} else {
		// The unconstrained minimizer gives a good initial point for L-BFGS.
		xInit = make([]float64, len(q))
		if err := solveSymmetric(solver.H, q, xInit); err != nil {
			for i := range xInit {
				xInit[i] = 0
			}
		}
		for i := range xInit {
			xInit[i] = -xInit[i]
		}
	}

	Ustar, err := minimizeLBFGS(xInit, objective, gradient, solver.Settings)
	if err != nil {
		// Keep the controller alive even if one numerical solve fails.
		Ustar = append([]float64(nil), xInit...)
	}
	u0 := append([]float64(nil), Ustar[:mIn]...)
	return u0, Ustar, nil
}

func minimizeLBFGS(x0 []float64, f func([]float64) float64, grad func([]float64, []float64), settings SolverSettings) ([]float64, error) {
	problem := optimize.Problem{
		Func: f,
		Grad: grad,
	}
	set := optimize.Settings{
		GradientThreshold: settings.Gtol,
		FuncEvaluations:   settings.MaxIter,
		MajorIterations:   settings.MaxIter,
	}
	res, _ := optimize.Minimize(problem, x0, &set, &optimize.LBFGS{})
	return res.X, nil
}

func matVecMul(a mat.Matrix, x []float64) []float64 {
	var y mat.VecDense
	y.MulVec(a, mat.NewVecDense(len(x), x))
	out := make([]float64, y.Len())
	copy(out, y.RawVector().Data)
	return out
}

func dot(a, b []float64) float64 {
	var sum float64
	for i := range a {
		sum += a[i] * b[i]
	}
	return sum
}

func solveSymmetric(H *mat.Dense, b []float64, dst []float64) error {
	n, _ := H.Dims()
	sym := mat.NewSymDense(n, nil)
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sym.SetSym(i, j, H.At(i, j))
		}
	}
	var chol mat.Cholesky
	_ = chol.Factorize(sym)
	bVec := mat.NewVecDense(len(b), b)
	var x mat.VecDense
	_ = chol.SolveVecTo(&x, bVec)
	copy(dst, x.RawVector().Data)
	return nil
}

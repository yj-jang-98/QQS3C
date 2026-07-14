package core

import "gonum.org/v1/gonum/mat"

type MPCProblem struct {
	// Lambda and Psi map x0 and the stacked control sequence U into the
	// predicted trajectory X. P, S, H are the condensed quadratic cost terms.
	A, B   *mat.Dense
	Q, R   *mat.Dense
	Qf     *mat.Dense
	N      int
	n, m   int
	Nm     int
	Lambda *mat.Dense
	Psi    *mat.Dense
	P      *mat.Dense
	S      *mat.Dense
	H      *mat.Dense
}

func NewMPCProblem(A, B, Q, R, Qf *mat.Dense, N int) *MPCProblem {
	mpc := &MPCProblem{
		A:  A,
		B:  B,
		Q:  Q,
		R:  R,
		Qf: Qf,
		N:  N,
	}
	n, _ := A.Dims()
	_, m := B.Dims()
	mpc.n = n
	mpc.m = m
	mpc.Nm = N * m
	mpc.Lambda, mpc.Psi = mpc.computePredictionMatrices()
	mpc.P, mpc.S, mpc.H = mpc.computeCostMatrices()
	return mpc
}

func (m *MPCProblem) InputDim() int {
	return m.m
}

func (m *MPCProblem) StackedInputDim() int {
	return m.Nm
}

func (m *MPCProblem) computePredictionMatrices() (*mat.Dense, *mat.Dense) {
	n, mIn, N := m.n, m.m, m.N
	A := m.A
	B := m.B

	A_powers := make([]*mat.Dense, N+1)
	A_powers[0] = Identity(n)
	// Precompute A^k once so the horizon expansion does not keep multiplying
	// the same matrices during controller initialization.
	for k := 1; k <= N; k++ {
		ap := mat.NewDense(n, n, nil)
		ap.Mul(A_powers[k-1], A)
		A_powers[k] = ap
	}

	Lambda := mat.NewDense(N*n, n, nil)
	// Lambda stacks [A; A^2; ...; A^N].
	for i := 0; i < N; i++ {
		copyBlock(Lambda, i*n, 0, A_powers[i+1])
	}

	Psi := mat.NewDense(N*n, N*mIn, nil)
	// Psi is block lower-triangular because each future state depends only on
	// current and earlier inputs in the stacked control sequence.
	for i := 0; i < N; i++ {
		row := i * n
		for j := 0; j <= i; j++ {
			col := j * mIn
			block := mat.NewDense(n, mIn, nil)
			block.Mul(A_powers[i-j], B)
			copyBlock(Psi, row, col, block)
		}
	}

	return Lambda, Psi
}

func (m *MPCProblem) computeCostMatrices() (*mat.Dense, *mat.Dense, *mat.Dense) {
	n, N := m.n, m.N
	// QBar and RBar are the horizon-stacked state/input penalties.
	QBar := BlockDiagRepeat(m.Q, N)
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			QBar.Set((N-1)*n+i, (N-1)*n+j, m.Qf.At(i, j))
		}
	}
	RBar := BlockDiagRepeat(m.R, N)

	var tmp mat.Dense
	tmp.Mul(QBar, m.Lambda)
	var P mat.Dense
	P.Mul(m.Lambda.T(), &tmp)
	P.Add(&P, m.Q)

	var tmp2 mat.Dense
	tmp2.Mul(QBar, m.Psi)
	var S mat.Dense
	S.Mul(m.Lambda.T(), &tmp2)
	S.Scale(2.0, &S)

	var tmp3 mat.Dense
	tmp3.Mul(m.Psi.T(), &tmp2)
	var H mat.Dense
	H.Add(&tmp3, RBar)
	H.Scale(2.0, &H)

	return &P, &S, &H
}

func (m *MPCProblem) BuildConstraintMatrices(Gx *mat.Dense, hx []float64, Gu *mat.Dense, hu []float64) (*mat.Dense, func([]float64) []float64) {
	N := m.N
	// The online solver works in condensed coordinates, so the per-step box
	// constraints are stacked once into G U <= h(x0).
	GxBar := BlockDiagRepeat(Gx, N)
	hxBar := TileVector(hx, N)
	GuBar := BlockDiagRepeat(Gu, N)
	huBar := TileVector(hu, N)

	var GTop mat.Dense
	GTop.Mul(GxBar, m.Psi)
	var LTop mat.Dense
	LTop.Mul(GxBar, m.Lambda)

	G := VStack(&GTop, GuBar)
	hFunc := func(x0 []float64) []float64 {
		Lx := MatVecMul(&LTop, x0)
		hTop := VecSub(hxBar, Lx)
		out := make([]float64, len(hTop)+len(huBar))
		copy(out, hTop)
		copy(out[len(hTop):], huBar)
		return out
	}
	return G, hFunc
}

func copyBlock(dst *mat.Dense, rowOffset, colOffset int, src mat.Matrix) {
	r, c := src.Dims()
	for i := 0; i < r; i++ {
		for j := 0; j < c; j++ {
			dst.Set(rowOffset+i, colOffset+j, src.At(i, j))
		}
	}
}

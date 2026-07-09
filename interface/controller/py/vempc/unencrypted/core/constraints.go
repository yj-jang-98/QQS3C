package core

import "gonum.org/v1/gonum/mat"

type ConstraintPenalty struct {
	G              *mat.Dense
	HFunc          func([]float64) []float64
	Mode           string
	HasConstraints bool
	P              int
}

func NewConstraintPenalty(G *mat.Dense, hFunc func([]float64) []float64, mode string) *ConstraintPenalty {
	r, _ := G.Dims()
	return &ConstraintPenalty{
		G:              G,
		HFunc:          hFunc,
		Mode:           mode,
		HasConstraints: true,
		P:              r,
	}
}

func (c *ConstraintPenalty) ConstraintResidualMat(U *mat.Dense, x0 []float64) *mat.Dense {
	r, _ := U.Dims()
	res := mat.NewDense(r, c.P, nil)
	c.ConstraintResidualMatInto(res, U, x0)
	return res
}

func (c *ConstraintPenalty) IsFeasibleMat(U *mat.Dense, x0 []float64, tol float64) []bool {
	res := c.ConstraintResidualMat(U, x0)
	r, _ := res.Dims()
	out := make([]bool, r)
	CountFeasibleResiduals(res, out, tol)
	return out
}

func (c *ConstraintPenalty) ConstraintResidualMatInto(dst *mat.Dense, U *mat.Dense, x0 []float64) {
	hVal := c.HFunc(x0)
	dst.Mul(U, c.G.T())
	raw := dst.RawMatrix()
	for i := 0; i < raw.Rows; i++ {
		row := raw.Data[i*raw.Stride : i*raw.Stride+raw.Cols]
		for j := 0; j < raw.Cols; j++ {
			row[j] -= hVal[j]
		}
	}
}

func CountFeasibleResiduals(residuals *mat.Dense, out []bool, tol float64) int {
	raw := residuals.RawMatrix()
	count := 0
	for i := 0; i < raw.Rows; i++ {
		row := raw.Data[i*raw.Stride : i*raw.Stride+raw.Cols]
		feasible := true
		for j := 0; j < raw.Cols; j++ {
			if row[j] > tol {
				feasible = false
				break
			}
		}
		out[i] = feasible
		if feasible {
			count++
		}
	}
	return count
}

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
	hVal := c.HFunc(x0)
	var res mat.Dense
	res.Mul(U, c.G.T())
	r, p := res.Dims()
	for i := 0; i < r; i++ {
		for j := 0; j < p; j++ {
			res.Set(i, j, res.At(i, j)-hVal[j])
		}
	}
	return &res
}

func (c *ConstraintPenalty) IsFeasibleMat(U *mat.Dense, x0 []float64, tol float64) []bool {
	res := c.ConstraintResidualMat(U, x0)
	r, p := res.Dims()
	out := make([]bool, r)
	for i := 0; i < r; i++ {
		feasible := true
		for j := 0; j < p; j++ {
			if res.At(i, j) > tol {
				feasible = false
				break
			}
		}
		out[i] = feasible
	}
	return out
}

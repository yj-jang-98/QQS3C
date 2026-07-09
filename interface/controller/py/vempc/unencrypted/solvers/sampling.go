package solvers

import "qqs3c_vempc/core"

func SampleVariationalControl(
	x0 []float64,
	variational *core.VariationalMPC,
	penalty *core.ConstraintPenalty,
	K int,
	chebCoeffs []float64,
	chebBound float64,
	chebEta float64,
	chebClip bool,
	seed int64,
) ([]float64, []float64, float64, int) {
	// U contains K candidate stacked control sequences drawn from the tilted
	// Gaussian defined by the variational reformulation.
	U := variational.SampleKappaTilde(x0, K, seed)

	// This exact feasible count is for diagnostics only; the controller itself
	// uses the polynomial surrogate weights below.
	feasible := penalty.IsFeasibleMat(U, x0, 1e-6)
	trueAccept := 0
	for _, f := range feasible {
		if f {
			trueAccept++
		}
	}

	weights, wSum := variational.ComputeWeights(U, x0, core.WeightOptions{
		ChebCoeffs: chebCoeffs,
		ChebBound:  chebBound,
		ChebEta:    chebEta,
		ChebClip:   chebClip,
		Eps:        1e-12,
	})

	_, Nm := U.Dims()
	UHat := make([]float64, Nm)
	if wSum == 0 {
		// Return zeros instead of NaNs so the outer loop can decide how to handle
		// a fully collapsed sample set.
		mIn := variational.MPC.InputDim()
		u0 := make([]float64, mIn)
		return u0, UHat, 0.0, trueAccept
	}
	for i := 0; i < K; i++ {
		for j := 0; j < Nm; j++ {
			UHat[j] += weights[i] * U.At(i, j)
		}
	}

	mIn := variational.MPC.InputDim()
	// Only the first input in the stacked sequence is applied to the plant.
	u0 := append([]float64(nil), UHat[:mIn]...)
	return u0, UHat, wSum, trueAccept
}

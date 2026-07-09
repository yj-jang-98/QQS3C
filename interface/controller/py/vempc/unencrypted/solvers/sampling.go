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
	workspace := core.NewVariationalWorkspace(variational, K)
	return SampleVariationalControlWithWorkspace(
		x0,
		variational,
		penalty,
		chebCoeffs,
		chebBound,
		chebEta,
		chebClip,
		seed,
		workspace,
	)
}

func SampleVariationalControlWithWorkspace(
	x0 []float64,
	variational *core.VariationalMPC,
	penalty *core.ConstraintPenalty,
	chebCoeffs []float64,
	chebBound float64,
	chebEta float64,
	chebClip bool,
	seed int64,
	workspace *core.VariationalWorkspace,
) ([]float64, []float64, float64, int) {
	// U contains K candidate stacked control sequences drawn from the tilted
	// Gaussian defined by the variational reformulation.
	U := variational.SampleKappaTildeInto(workspace, x0, seed)

	// This exact feasible count is for diagnostics only; the controller itself
	// uses the polynomial surrogate weights below.
	penalty.ConstraintResidualMatInto(workspace.Residuals, U, x0)
	trueAccept := core.CountFeasibleResiduals(workspace.Residuals, workspace.Feasible, 1e-6)

	wSum := variational.ComputeWeightsFromResidualsInto(workspace.Residuals, core.WeightOptions{
		ChebCoeffs: chebCoeffs,
		ChebBound:  chebBound,
		ChebEta:    chebEta,
		ChebClip:   chebClip,
		Eps:        1e-12,
	}, workspace.Surrogate, workspace.LogW, workspace.Weights)

	_, Nm := U.Dims()
	UHat := workspace.UHat[:Nm]
	for i := 0; i < Nm; i++ {
		UHat[i] = 0.0
	}
	if wSum == 0 {
		// Return zeros instead of NaNs so the outer loop can decide how to handle
		// a fully collapsed sample set.
		mIn := variational.MPC.InputDim()
		u0 := workspace.U0[:mIn]
		for i := 0; i < mIn; i++ {
			u0[i] = 0.0
		}
		return u0, UHat, 0.0, trueAccept
	}

	raw := U.RawMatrix()
	for i := 0; i < raw.Rows; i++ {
		w := workspace.Weights[i]
		if w == 0.0 {
			continue
		}
		row := raw.Data[i*raw.Stride : i*raw.Stride+Nm]
		for j := 0; j < Nm; j++ {
			UHat[j] += w * row[j]
		}
	}

	mIn := variational.MPC.InputDim()
	// Only the first input in the stacked sequence is applied to the plant.
	u0 := workspace.U0[:mIn]
	copy(u0, UHat[:mIn])
	return u0, UHat, wSum, trueAccept
}

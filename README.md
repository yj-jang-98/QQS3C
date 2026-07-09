# QQS3C VEMPC Controller

This README documents the current implementation in `QQS3C/interface/controller/py/vempc`.
It focuses on:

- what `ctrl_vempc.py` does
- how it interacts with the plant
- which variational MPC parameters are currently used
- how to run the controller in simulation or with the hardware-side plant code

## Folder Structure

- `interface/controller/py/vempc/config.py`
  Defines the controller configuration exported from Python to the Go engine.
- `interface/controller/py/vempc/model.py`
  Starts, stops, and communicates with the Go VEMPC engine.
- `interface/controller/py/vempc/ctrl_vempc.py`
  TCP client that talks to the QQS3C plant and forwards measurements to the Go engine.
- `interface/controller/py/vempc/unencrypted/`
  Local Go module that implements the observer, condensed MPC, and variational MPC engine.
- `interface/controller/py/vempc/runtime/controller_config.json`
  Generated config file written by Python before the Go engine starts.
- `interface/controller/py/vempc/results/`
  Overwritten on every run with:
  - `cycles.csv`
  - `summary.json`
  - `qube_signals.png`

## Current Controller Type

The current default backend is **variational MPC**.

This is set in `interface/controller/py/vempc/config.py`:

- `backend = "variational"`

If you change that field to `"standard"`, the same Python bridge will run the deterministic condensed MPC solver instead.

## Current Variational MPC Configuration

All current parameters are defined in `interface/controller/py/vempc/config.py`.

### Sampling and horizon

- `sample_time = 0.02`
  The controller runs at 50 Hz.
- `horizon = 15`
  The MPC horizon is 15 steps.
  With `dt = 0.02`, this corresponds to 0.30 s of look-ahead.

### QQS3C plant/observer model

The Go engine does not rebuild a model from physical parameters.
It uses the discrete QQS3C model exported directly from Python:

- `A`
  4x4 discrete state matrix
- `B`
  4x1 discrete input matrix
- `C`
  2x4 output matrix
- `L`
  4x2 observer gain

This means the controller behavior follows the QQS3C identified model, not the separate IFAC QUBE parameter model.

### Cost

- `q_diag = (5000.0, 400.0, 1.0, 1.0)`
  State penalty diagonal.
  This strongly penalizes the first two states, especially the first one.
- `r_diag = (1.0,)`
  Input penalty diagonal.
- `qf_scale = 10.0`
  Terminal cost is `Qf = 10 * Q`.

### Constraints

- `alpha_max = radians(15.0)`
  State constraint for the pendulum angle used by the Go engine.
- `u_max = 15.0`
  Input magnitude bound used in MPC.

### Variational parameters

- `sigma0 = 6.0`
  Width of the prior covariance for the sampled stacked control sequence.
  Larger values spread samples more widely.
- `lambda_param = 0.75`
  Temperature parameter in the variational formulation.
  It controls how strongly the cost term shapes the tilted Gaussian.
- `k_samples = 512`
  Number of sampled candidate input sequences per control step.
- `cheb_order = 7`
  Degree of the Chebyshev polynomial used to approximate ReLU in the constraint-weighting stage.
- `cheb_bound = 15.0`
  Scaling bound for the polynomial surrogate.
- `cheb_eta = 1.0`
  Exponential penalty scaling on surrogate constraint violations.
- `cheb_clip = True`
  Clips the normalized input and negative surrogate output in the weighting pipeline.

### Initial estimate

- `x0 = (0.0, 0.0, 0.0, 0.0)`
  Initial observer state inside the Go engine.

## What `ctrl_vempc.py` Does

`ctrl_vempc.py` is the outer controller process. It does not implement MPC directly.
Its job is to bridge:

- the QQS3C plant TCP protocol
- the local Go VEMPC engine

The high-level flow is:

1. Create or reuse `results/`.
2. Start `GoVEMPCController()`.
3. `GoVEMPCController()` writes `runtime/controller_config.json`.
4. `GoVEMPCController()` builds `unencrypted/vempc_engine.exe` if needed.
5. `GoVEMPCController()` launches the Go process and keeps it alive.
6. `ctrl_vempc.py` opens a TCP client to the plant at `localhost:9999`.
7. On every `"run"` signal:
   - receive `y0`
   - receive `y1`
   - send `{"type":"measure","y":[y0,y1]}` to the Go process over `stdin`
   - receive a JSON response from `stdout`
   - extract `u`
   - send `u` back to the plant over TCP
   - log timing and diagnostics
8. On `"end"`:
   - stop the loop
   - write `cycles.csv`, `summary.json`, and `qube_signals.png`
   - print mean and variance of control-cycle time

## How the Go Engine Works

The Go engine lives in `interface/controller/py/vempc/unencrypted/main.go`.

For each measurement update:

1. Correct the observer state with the measured output.
2. Build the current MPC state vector `x_hat`.
3. If backend is `"standard"`:
   - solve condensed MPC with the soft-constrained L-BFGS solver
4. If backend is `"variational"`:
   - sample `K = 512` candidate stacked input sequences
   - compute surrogate constraint weights
   - form a weighted average
   - apply only the first control input
5. Predict the observer forward with the applied input.
6. Return a JSON response containing:
   - `u`
   - `x_hat`
   - `w_sum` and `accept_num` for variational mode

## Plant Interaction

The plant side is the TCP server. The controller side is the TCP client.

### Simulation plant

`interface/plant/py/simulation/plant.py` does the following:

1. Send `"run"`
2. Send `y[0]`
3. Send `y[1]`
4. Wait for one scalar control input `u`
5. Update the simulated plant
6. Repeat
7. Send `"end"` after the run finishes

### Hardware plant

`interface/plant/py/hardware/plant.py` and `plant_with_swing_up.py` use the same TCP handshake:

1. Send `"run"`
2. Send measured outputs
3. Receive `u`
4. Apply actuator logic and saturation
5. Repeat
6. Send `"end"` when the experiment ends

In `plant_with_swing_up.py`, the VEMPC controller is only used after the swing-up and switching logic hand over control to the external controller.

## Results Written by the Controller

Every time `ctrl_vempc.py` completes a run, it overwrites:

- `interface/controller/py/vempc/results/cycles.csv`
- `interface/controller/py/vempc/results/summary.json`
- `interface/controller/py/vempc/results/qube_signals.png`

### `cycles.csv`

Contains one row per control cycle:

- `step`
- `y0`
- `y1`
- `u`
- `cycle_ms`
- `w_sum`
- `accept_num`
- `x_hat_0`
- `x_hat_1`
- `x_hat_2`
- `x_hat_3`

### `summary.json`

Contains:

- `backend`
- `steps`
- `avg_cycle_ms`
- `var_cycle_ms`
- `results_dir`

### `qube_signals.png`

Contains four plots:

- `y0`
- `y1`
- `u`
- control-cycle time in milliseconds

These plots come from the controller log, not from Quanser's live `Scope` windows.

## How to Run

## Requirements

- Python environment with:
  - `numpy`
  - `matplotlib`
- Go installed and available on `PATH`
- `tcp_protocol_client.py` / `tcp_protocol_server.py` reachable through `PYTHONPATH`

For hardware:

- Quanser `pal` Python dependencies installed

## Set `PYTHONPATH`

From the repository root, set:

```powershell
$env:PYTHONPATH = "QQS3C/communication/py"
```

This is required because the controller and plant scripts import `tcp_protocol_client` / `tcp_protocol_server` directly.

## Run in Simulation

Terminal 1:

```powershell
python QQS3C/interface/plant/py/simulation/plant.py
```

Terminal 2:

```powershell
python QQS3C/interface/controller/py/vempc/ctrl_vempc.py
```

Expected behavior:

- the plant acts as TCP server
- the controller builds or reuses the Go engine
- the controller and plant exchange measurements and inputs
- when the simulation ends, the controller writes fresh files into `interface/controller/py/vempc/results`

## Run with Hardware Swing-Up Front-End

Terminal 1:

```powershell
python QQS3C/interface/plant/py/hardware/plant_with_swing_up.py
```

Terminal 2:

```powershell
python QQS3C/interface/controller/py/vempc/ctrl_vempc.py
```

In this mode:

- the plant-side script handles swing-up and switching
- once it enters the external-controller phase, it starts the same TCP `"run"` / measurement / `u` exchange
- the VEMPC controller responds exactly as in simulation mode

## Notes

- The function name `full_state_feedback()` in `ctrl_vempc.py` is historical.
  The current controller is not full-state feedback; it is a TCP bridge to the Go MPC engine.
- The default backend is variational MPC.
  To switch to standard MPC, change `backend` in `interface/controller/py/vempc/config.py`.
- The first controller run may take longer because the Go engine may be rebuilt.

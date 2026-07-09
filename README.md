# QQS3C VEMPC

`QQS3C/interface/controller/py/vempc` is a Python-to-Go bridge for the QQS3C controller.

- `config.py`: QQS3C model, MPC tuning, and plot settings
- `model.py`: builds or reuses the Go engine and exchanges JSON with it
- `ctrl_vempc.py`: talks to the plant over TCP and saves results
- `unencrypted/`: Go implementation of the observer and MPC solver

## Current Setup

- Default backend: `variational`
- Sample time: `0.02 s`
- Horizon: `15`
- Cost: `Q = diag(5000, 400, 1, 1)`, `R = diag(1)`
- Constraints: `|alpha| <= 15 deg`, `|u| <= 15`
- Variational parameters: `sigma0 = 6`, `lambda = 0.75`, `K = 512`, `cheb_order = 7`

The Go side uses the discrete QQS3C matrices exported from `config.py`. It does not rebuild a separate physical model.

## Flow

`ctrl_vempc.py` does this:

1. Write `runtime/controller_config.json`
2. Start `unencrypted/vempc_engine`
3. Connect to the plant at `localhost:9999`
4. For each `"run"` message:
   - receive `y0`, `y1`
   - send them to Go
   - receive `u`
   - send `u` back to the plant
5. On `"end"`, save results in `interface/controller/py/vempc/results`

The current function name `full_state_feedback()` is historical. The controller itself is the Go MPC engine.

## Results

Each run overwrites:

- `results/cycles.csv`
- `results/summary.json`
- `results/qube_signals.png`

`qube_signals.png` contains:

- `y0`
- `y1`
- `u`
- control-cycle time

When `plot_phase_mode = "hardware_swing_up"`, the figure also marks:

- swing-up before controller handoff
- initial plant-side state-feedback interval
- intervals where the plant is outside the external-controller region

These phase labels match the logic in `interface/plant/py/hardware/plant_with_swing_up.py`.

## Run

Set:

```powershell
$env:PYTHONPATH = "QQS3C/communication/py"
```

Simulation:

```powershell
python QQS3C/interface/plant/py/simulation/plant.py
python QQS3C/interface/controller/py/vempc/ctrl_vempc.py
```

Hardware swing-up front-end:

```powershell
python QQS3C/interface/plant/py/hardware/plant_with_swing_up.py
python QQS3C/interface/controller/py/vempc/ctrl_vempc.py
```

If you want standard MPC instead of variational MPC, change `backend` in `config.py` to `"standard"`.

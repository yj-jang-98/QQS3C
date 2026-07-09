import tcp_protocol_client as tcc
import csv
import json
import shutil
import time
from pathlib import Path

# init tcp host and port
HOST = 'localhost'
PORT = 9999

# get model description
import model
from config import RESULTS_DIR

# get other tools
import matplotlib.pyplot as plt
import numpy as np
from matplotlib.patches import Patch

def _prepare_results_dir():
    RESULTS_DIR.mkdir(parents=True, exist_ok=True)
    for child in RESULTS_DIR.iterdir():
        if child.is_dir() and child.name.startswith("run_"):
            shutil.rmtree(child)
    return RESULTS_DIR

def _save_results(results_dir: Path, rows: list[dict], summary: dict):
    csv_path = results_dir / "cycles.csv"
    summary_path = results_dir / "summary.json"

    fieldnames = [
        "step",
        "y0",
        "y1",
        "u",
        "cycle_ms",
        "w_sum",
        "accept_num",
        "x_hat_0",
        "x_hat_1",
        "x_hat_2",
        "x_hat_3",
    ]
    with csv_path.open("w", newline="", encoding="utf-8") as fh:
        writer = csv.DictWriter(fh, fieldnames=fieldnames)
        writer.writeheader()
        writer.writerows(rows)

    with summary_path.open("w", encoding="utf-8") as fh:
        json.dump(summary, fh, indent=2)

def _mask_to_spans(mask: np.ndarray) -> list[tuple[float, float]]:
    spans = []
    start = None
    for idx, active in enumerate(mask.tolist()):
        if active and start is None:
            start = idx
        elif not active and start is not None:
            spans.append((start - 0.5, idx - 0.5))
            start = None
    if start is not None:
        spans.append((start - 0.5, len(mask) - 0.5))
    return spans

def _add_phase_overlay(axes, phase_ax, rows: list[dict], config):
    phase_colors = {
        "swing_up": "#bdbdbd",
        "state_feedback": "#f6c85f",
        "outside_controller": "#ff6f69",
    }

    row_count = len(rows)
    x_right = max(row_count - 0.5, 0.5)
    x_left = -0.5

    if config.plot_phase_mode == "hardware_swing_up":
        x_left = -float(max(1, config.swing_up_display_steps))

        # The controller log starts only after handoff from the plant-side
        # swing-up logic, so this left band is schematic rather than timed.
        phase_ax.axvspan(x_left, -0.5, color=phase_colors["swing_up"], alpha=0.35, hatch="//")
        phase_ax.text(
            0.5 * (x_left - 0.5),
            0.5,
            "swing-up\nbefore step 0",
            ha="center",
            va="center",
            fontsize=9,
        )

        state_feedback_end = min(float(config.state_feedback_steps) - 0.5, x_right)
        if state_feedback_end > -0.5:
            phase_ax.axvspan(-0.5, state_feedback_end, color=phase_colors["state_feedback"], alpha=0.35)
            for ax in axes:
                ax.axvspan(-0.5, state_feedback_end, color=phase_colors["state_feedback"], alpha=0.12)
            phase_ax.text(
                0.5 * (state_feedback_end - 0.5),
                0.5,
                "state feedback",
                ha="center",
                va="center",
                fontsize=9,
            )

        outside_mask = np.array(
            [
                idx >= config.state_feedback_steps
                and abs(np.degrees(row["y1"])) >= config.outside_controller_angle_deg
                for idx, row in enumerate(rows)
            ],
            dtype=bool,
        )
        outside_spans = _mask_to_spans(outside_mask)
        outside_labeled = False
        for start, end in outside_spans:
            phase_ax.axvspan(start, end, color=phase_colors["outside_controller"], alpha=0.35)
            for ax in axes:
                ax.axvspan(start, end, color=phase_colors["outside_controller"], alpha=0.12)
            if not outside_labeled and end - start >= 3.0:
                phase_ax.text(
                    0.5 * (start + end),
                    0.5,
                    "outside controller",
                    ha="center",
                    va="center",
                    fontsize=9,
                )
                outside_labeled = True

        legend_handles = [
            Patch(facecolor=phase_colors["swing_up"], alpha=0.35, hatch="//", label="Swing-up"),
            Patch(facecolor=phase_colors["state_feedback"], alpha=0.35, label="State feedback"),
            Patch(facecolor=phase_colors["outside_controller"], alpha=0.35, label="Outside controller"),
        ]
        phase_ax.legend(handles=legend_handles, loc="upper right", ncol=3, fontsize=8, frameon=True)
    else:
        phase_ax.text(0.5 * (x_left + x_right), 0.5, "controller-active interval", ha="center", va="center", fontsize=9)

    phase_ax.set_xlim(x_left, x_right)
    phase_ax.set_ylim(0.0, 1.0)
    phase_ax.set_yticks([])
    phase_ax.set_ylabel("phase")
    phase_ax.grid(False)
    phase_ax.set_xticks([])

def _save_plots(results_dir: Path, rows: list[dict], backend_name: str, config):
    if not rows:
        return

    t = np.arange(len(rows), dtype=float)
    y0 = np.array([row["y0"] for row in rows], dtype=float)
    y1 = np.array([row["y1"] for row in rows], dtype=float)
    u = np.array([row["u"] for row in rows], dtype=float)
    cycle_ms = np.array([row["cycle_ms"] for row in rows], dtype=float)

    fig, axes = plt.subplots(
        5,
        1,
        figsize=(11, 10),
        sharex=True,
        gridspec_kw={"height_ratios": [0.45, 1.0, 1.0, 1.0, 1.0]},
    )
    phase_ax = axes[0]
    signal_axes = axes[1:]

    signal_axes[0].plot(t, y0)
    signal_axes[0].set_ylabel("y0")
    signal_axes[0].grid(True)

    signal_axes[1].plot(t, y1)
    signal_axes[1].set_ylabel("y1")
    signal_axes[1].grid(True)

    signal_axes[2].plot(t, u)
    signal_axes[2].set_ylabel("u")
    signal_axes[2].grid(True)

    signal_axes[3].plot(t, cycle_ms)
    signal_axes[3].set_ylabel("ms")
    signal_axes[3].set_xlabel("controller step")
    signal_axes[3].grid(True)

    _add_phase_overlay(signal_axes, phase_ax, rows, config)

    fig.suptitle(f"QQS3C VEMPC Run ({backend_name})")
    fig.tight_layout(rect=[0.0, 0.0, 1.0, 0.97])
    fig.savefig(results_dir / "qube_signals.png")
    plt.close(fig)

def full_state_feedback():
    run_signal = True
    results_dir = _prepare_results_dir()

    # Keep the plant-facing interface identical to the other QQS3C controllers:
    # the plant sends two outputs, and this controller replies with one input.
    y = np.array([[0],
                  [0]], dtype=float)
    rows = []
    cycle_ms = []
    step = 0

    with model.GoVEMPCController() as controller, tcc.tcp_client(HOST, PORT) as tccp:
        backend_name = controller.config.backend
        plot_config = controller.config
        while run_signal:
            # running signal send for controller
            _, signal = tccp.recv() # Waiting for a plant-side signal

            if signal == "run":
                # get plant output
                _, y0 = tccp.recv()
                _, y1 = tccp.recv()
                y[0, 0] = y0
                y[1, 0] = y1

                # Python forwards only the latest measurement. The Go engine
                # performs observer correction and computes the new MPC action.
                start_ns = time.perf_counter_ns()
                response = controller.compute_control_detail(y.reshape(-1))
                x_hat = list(response.get("x_hat", []))
                while len(x_hat) < 4:
                    x_hat.append(float("nan"))
                u = float(response["u"])
                tccp.send(u)

                elapsed_ms = (time.perf_counter_ns() - start_ns) / 1_000_000.0
                cycle_ms.append(elapsed_ms)
                rows.append({
                    "step": step,
                    "y0": float(y0),
                    "y1": float(y1),
                    "u": u,
                    "cycle_ms": elapsed_ms,
                    "w_sum": response.get("w_sum", ""),
                    "accept_num": response.get("accept_num", ""),
                    "x_hat_0": x_hat[0],
                    "x_hat_1": x_hat[1],
                    "x_hat_2": x_hat[2],
                    "x_hat_3": x_hat[3],
                })
                step += 1

            elif signal == "end":
                # end of loop signal get
                run_signal = False
                break

    if cycle_ms:
        avg_ms = float(np.mean(cycle_ms))
        var_ms = float(np.var(cycle_ms))
    else:
        avg_ms = 0.0
        var_ms = 0.0

    summary = {
        "backend": backend_name,
        "steps": len(rows),
        "avg_cycle_ms": avg_ms,
        "var_cycle_ms": var_ms,
        "plot_phase_mode": plot_config.plot_phase_mode,
        "results_dir": str(results_dir),
    }
    _save_results(results_dir, rows, summary)
    _save_plots(results_dir, rows, backend_name, plot_config)

    print(f"VEMPC results saved to {results_dir}")
    print(f"Control-cycle mean: {avg_ms:.6f} ms")
    print(f"Control-cycle variance: {var_ms:.6f} ms^2")

def main():
    full_state_feedback()

if __name__ == "__main__":
    main()

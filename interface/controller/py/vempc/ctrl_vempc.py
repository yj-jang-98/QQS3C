import tcp_protocol_client as tcc

# init tcp host and port
HOST = 'localhost'
PORT = 9999

# get model description
import model

# get other tools
import numpy as np

def full_state_feedback():
    run_signal = True

    # input/output initialization
    y = np.array([[0],
                  [0]], dtype=float)

    with model.GoVEMPCController() as controller, tcc.tcp_client(HOST, PORT) as tccp:
        while run_signal:
            # running signal send for controller
            _, signal = tccp.recv() # Waiting for a plant-side signal

            if signal == "run":
                # get plant output
                _, y0 = tccp.recv()
                _, y1 = tccp.recv()
                y[0, 0] = y0
                y[1, 0] = y1

                u = controller.compute_control(y.reshape(-1))
                tccp.send(float(u))

            elif signal == "end":
                # end of loop signal get
                run_signal = False
                break

def main():
    full_state_feedback()

if __name__ == "__main__":
    main()

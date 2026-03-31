import tcp_protocol_client as tcc

# init tcp host and port
HOST = 'localhost'
PORT = 9999

# get model description
import model

# get other tools
import numpy as np

def integer_state_matrix():
    # set simulation(this section have to set same with plant)
    # alternatively this controller match sampling_time with MATLAB code(for transpose matrix T, cf. model.py this folder)
    sampling_time = 0.02
    run_signal = True

    # get model from model description file
    obs = model.obs(sampling_time)
    print(f"controller's matrix F: \n{obs.F}")
    print(f"controller's matrix G: \n{obs.G}")
    print(f"controller's matrix H: \n{obs.H}\n")

    intmat = model.intmat(obs.F, obs.G, obs.H, obs.J, obs.ts)
    print(f"controller's converted matrix F: \n{intmat.F_cv}")
    print(f"controller's converted matrix R: \n{intmat.R_cv}")
    print(f"controller's converted matrix G: \n{intmat.G_cv}")
    print(f"controller's converted matrix H: \n{intmat.H_cv}\n")

    intmat_q = model.intmat_q(intmat.F_cv, intmat.G_cv, intmat.H_cv, intmat.R_cv)

    # set quantized level and quantize matrix
    intmat_q.set_level(1000, 1000)
    intmat_q.quantize()

    # print matrix of F_q, G_q, H_q, and R_q
    print(f"print F_q, G_q, H_q, and R_q: ")
    print(intmat_q.F_q)
    print(intmat_q.G_q)
    print(intmat_q.H_q)
    print(intmat_q.R_q)

    # input/output initialization
    y = np.array([[0],
                  [0]], dtype=float)
    u = np.array([[0]], dtype=float)

    with tcc.tcp_client(HOST, PORT) as tccp:
        while run_signal:
            # running signal send for controller
            _, signal = tccp.recv()

            if signal == "run":
                # get plant output
                _, y0 = tccp.recv()
                _, y1 = tccp.recv()
                y[0, 0] = y0
                y[1, 0] = y1

                # send control input data
                tccp.send(u[0, 0])

                # state update and generate output
                intmat_q.state_update(y, u)
                u = intmat_q.get_output()
            elif signal == "end":
                # end of loop signal get
                run_signal = False
                break

def main():
    integer_state_matrix()

if __name__ == "__main__":
    main()

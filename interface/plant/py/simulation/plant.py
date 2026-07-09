import tcp_protocol_server as tcs

# init tcp host and port
HOST = '0.0.0.0'
PORT = 9999

# get model description
import model

# get other tools
import numpy as np
import matplotlib.pyplot as plt
import time

def main():
    # set simulation(this section have to set same with controller)
    sampling_time = 0.02
    max_time = 10
    max_iter = int(max_time / sampling_time)

    # get model from model description file
    plant = model.rotpen(sampling_time)

    # set state initial value
    plant.set_init(np.array([[-0.3],
                             [-0.2],
                             [0],
                             [0]], dtype=float))

    # input/output initialization
    y = np.array([[0],
                  [0]], dtype=float)
    u = np.array([[0]], dtype=float)

    # state and outupt memory initialization for history plot
    time_stack = np.zeros((max_iter, 1))
    y_his = np.zeros((max_iter, 2))

    with tcs.tcp_server(HOST, PORT) as tcsp:
        for i in range(max_iter):
            # start time set for measurment
            start_clock = time.perf_counter_ns()

            # running signal send for controller
            tcsp.send("run")

            # get plant output, send data, and save data
            y = plant.get_output()
            tcsp.send(y[0, 0])
            tcsp.send(y[1, 0])
            time_stack[i, 0] = i * sampling_time
            y_his[i, 0] = y[0, 0]
            y_his[i, 1] = y[1, 0]

            # get control input from controller
            _, uk = tcsp.recv()
            u[0, 0] = uk

            # plant state update
            plant.state_update(u)

            # end time set for measurment and calculation duration(transform unit to ms)
            end_clock = time.perf_counter_ns()
            duration = (end_clock - start_clock) / 1000000000

            # sleep to satisfy sampling time(if you want to save your time, comment out the sentence below)
            # time.sleep(sampling_time - duration)
        
        # endding signal send for controller
        tcsp.send("end")

        # draw plot of plant output y value
        fig, axes = plt.subplots(2, 1)
        axes[0].plot(time_stack, y_his[:,0])
        axes[0].set_title('position')
        axes[0].grid(True)
        axes[1].plot(time_stack, y_his[:,1])
        axes[1].set_title('angle')
        axes[1].grid(True)
        fig.suptitle('plant output')
        plt.tight_layout()
        plt.savefig('./result/plant output as sim.png')

if __name__ == "__main__":
    main()

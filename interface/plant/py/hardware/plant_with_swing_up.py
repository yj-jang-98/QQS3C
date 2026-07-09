from pal.products.qube import QubeServo2, QubeServo3
from pal.utilities.math import SignalGenerator, ddt_filter
from pal.utilities.scope import Scope

import tcp_protocol_server as tcs

# init tcp host and port
HOST = '0.0.0.0'
PORT = 9999

# get other tools
from threading import Thread
import signal
import time
import math
import control as ct
import numpy as np

# Thread hanlder initialization
global KILL_THREAD
KILL_THREAD = False
def sig_handler(*args):
    global KILL_THREAD
    KILL_THREAD = True
signal.signal(signal.SIGINT, sig_handler)

# simulation time and plotting set
simulationTime = 15 # will run for 15 seconds
color = np.array([0, 1, 0], dtype=np.float64)

scopePendulum = Scope(
    title='Pendulum encoder - alpha (rad)',
    timeWindow=10,
    xLabel='Time (s)',
    yLabel='Position (rad)')
scopePendulum.attachSignal(name='Pendulum - alpha (rad)',  width=1)

scopeBase = Scope(
    title='Base encoder - theta (rad)',
    timeWindow=10,
    xLabel='Time (s)',
    yLabel='Position (rad)')
scopeBase.attachSignal(name='Base - theta (rad)',  width=1)

scopeVoltage = Scope(
    title='Motor Voltage',
    timeWindow=10,
    xLabel='Time (s)',
    yLabel='Voltage (volts)')
scopeVoltage.attachSignal(name='Voltage',  width=1)

# control-system scenario
def control_loop():
    # interface setting #
    # ------------------------------------------------ #
    # qube version, using hardware, pendulum
    qubeversion = 3
    
    # if you want to use Ouanser Interactive Labs, you will change to 0
    hardware = 0
    
    pendulum = 1

    # frequency of system holder and sampler
    frequency = 50 # hz

    # for scope sampling rate
    countMax = frequency / 50
    count = 0

    # class initialization
    QubeClass = QubeServo3

    # for state estimation
    state_theta_dot = np.array([0,0], dtype=np.float64)
    state_alpha_dot = np.array([0,0], dtype=np.float64)

    # swing-up standing gate
    stand_run = False
    change_flag = False
    set_time = 0.0
    switching_time = 100.0 * 1/frequency # full-state feedback controller on plant run time (stabilize state which is perturbated by swing-up phase)
    transient_time = 0.0 * 1/frequency # stop update control input for transient phase rum time (set a initial value by stopping control input during a little moments)

    # gain of full-state(use initial part of swing-up)
    A = np.array([[0, 0, 1, 0],
                  [0, 0, 0, 1],
                  [0, 149.275096865093, -0.0130994260613721, 0],
                  [0, 261.609107366662, -0.0129471071536817, 0]], dtype=float)
    
    B = np.array([[0],
                  [0],
                  [55.6948386963098],
                  [55.0472242928643]], dtype=float)
    
    C = np.array([[1, 0, 0, 0],
                  [0, 1, 0, 0],
                  [0, 0, 1, 0],
                  [0, 0, 0, 1]], dtype=float)
    
    D = np.array([[0],
                  [0],
                  [0],
                  [0]], dtype=float)
    
    sys_c = ct.ss(A, B, C, D)
    sys_d = sys_c.sample((1 / frequency), method='zoh')

    Q_k = np.array([[5, 0, 0, 0],
                    [0, 1, 0, 0],
                    [0, 0, 1, 0],
                    [0, 0, 0, 1]], dtype=float)
    R_k = np.array([[1]], dtype=float)
    K, Sk, Ek = ct.dlqr(sys_d.A, sys_d.B, Q_k, R_k)

    # Physical Parameters
    mp = 0.024       
    Lp = 0.129       
    l = Lp / 2       
    g = 9.81         
    Jp = mp * (Lp**2) / 3  

    # Control Parameters
    E_ref = 0.0      
    mu = 150.0  

    # switching target angle
    angle = 10

    # reliable angle range (for hardware operate limit)
    angle_range = 20

    # describe #
    # ------------------------------------------------ #
    # instance of hardware model 
    with QubeClass(hardware=hardware, pendulum=pendulum, frequency=frequency) as myQube:
        # instance of tcp layer
        with tcs.tcp_server(HOST, PORT) as tcsp:
            startTime = 0
            timeStamp = 0
            def elapsed_time():
                return time.time() - startTime
            startTime = time.time()

            while timeStamp < simulationTime and not KILL_THREAD:
                if not stand_run:
                    # read sensor information
                    myQube.read_outputs()

                    # calc output
                    theta = myQube.motorPosition * -1
                    alpha_f =  myQube.pendulumPosition
                    alpha = np.mod(alpha_f, 2*np.pi) - np.pi
                    alpha_deg = abs(math.degrees(alpha))

                    # Calculate angular velocities with filter of 50 and 100 rad
                    theta_dot, state_theta_dot = ddt_filter(theta, state_theta_dot, 50, 1/frequency)
                    alpha_dot, state_alpha_dot = ddt_filter(alpha, state_alpha_dot, 100, 1/frequency)
                    states = np.array([theta, alpha, theta_dot, alpha_dot])

                    # Energy Calculation
                    E_pot = mp * g * l * (math.cos(alpha) - 1)
                    E_kin = 0.5 * Jp * (alpha_dot**2)
                    E_total = E_pot + E_kin
                    E_err = E_ref - E_total

                    # calc control rate
                    term = np.sign(alpha_dot * math.cos(alpha))
                    if abs(alpha_dot) < 0.05 and abs(math.cos(alpha)) < 0.1:
                        term = 0
                    u_raw = -1 * mu * E_err * term

                    if(alpha_deg > angle and (not change_flag)):
                        if(-theta > 0):
                            if(u_raw > 0):
                                voltage = 0.0
                            else:
                                voltage = u_raw
                        else:
                            if(u_raw < 0):
                                voltage = 0.0
                            else:
                                voltage = u_raw
                    else:
                        voltage = 1*np.dot(K, states)
        
                        if(not change_flag):
                            change_flag = True
                            set_time = timeStamp
                            stand_run = True
                    
                    # write commands
                    voltage = np.clip(voltage, -8, 8)
                    myQube.write_voltage(voltage)

                    print(f"control start: {stand_run}")
                else:
                    # running signal send for controller
                    tcsp.send("run")
    
                    # read sensor information
                    myQube.read_outputs()
    
                    # calc output
                    theta = myQube.motorPosition * -1
                    alpha_f =  myQube.pendulumPosition
                    alpha = np.mod(alpha_f, 2*np.pi) - np.pi
                    alpha_deg = alpha * 180 / np.pi

                    # Calculate angular velocities with filter of 50 and 100 rad
                    theta_dot, state_theta_dot = ddt_filter(theta, state_theta_dot, 50, 1/frequency)
                    alpha_dot, state_alpha_dot = ddt_filter(alpha, state_alpha_dot, 100, 1/frequency)
                    states = np.array([theta, alpha, theta_dot, alpha_dot])
    
                    # send plant output
                    tcsp.send(-theta)
                    tcsp.send(-alpha)
    
                    # get control input
                    _, u = tcsp.recv()
    
                    # swing up trasient responce cushioning
                    if(timeStamp - set_time < switching_time):
                        voltage = 1*np.dot(K, states)
                        print("control object: full-state on plant")
                    else:
                        if(timeStamp - set_time < switching_time + transient_time):
                            voltage = 0
                            print("control object: transient phase")
                        else:
                            # running range set
                            if abs(alpha_deg) < angle_range:
                                voltage = u
                            else:
                                voltage = 0
                            print("control object: outside controller")
                    
                    # write commands
                    voltage = np.clip(voltage, -15, 15)
                    myQube.write_voltage(voltage)

                # plot to scopes
                count += 1
                if count >= countMax:
                    scopePendulum.sample(timeStamp, -alpha)
                    scopeBase.sample(timeStamp, -theta)
                    scopeVoltage.sample(timeStamp,voltage)
                    count = 0

                timeStamp = elapsed_time()

            tcsp.send("end")

def main():
    thread_cl = Thread(target=control_loop)
    thread_cl.start()

    while thread_cl.is_alive() and (not KILL_THREAD):

        # This must be called regularly or the scope windows will freeze
        # Must be called in the main thread.
        Scope.refreshAll()
        time.sleep(0.01)

    input('Press the enter key to exit.')

if __name__ == "__main__":
    main()

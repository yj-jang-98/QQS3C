#define _USE_MATH_DEFINES
#include <iostream>
#include <string>
#include <cmath>
#include "tcp_protocol_server_windows.h"
#include "hil.h"
#include "quanser_timer.h"
using namespace std;

const string host = "0.0.0.0";
const int port = 9999;

int main()
{
    t_card board;
    t_error result;

    /// If you want to use hardware, activate below
    // result = hil_open("qube_servo3_usb", "0", &board);
    // if (result < 0)
    // {
    //     cout << "failure to connect hardware" << endl;
    //        return -1;
    // }
   
    /// If you want to use virtual environment activate below
    result = hil_open("qube_servo3_usb", "0@tcpip://localhost:18923?nagle='off'", &board);
    if (result < 0)
    {
        cout << "failure to connect QLab" << endl;
           return -1;
    }

    // simulation_time is total run time, sample_time is sample_time.
    int simulation_time = 30;
    double sample_time = 0.02;
    t_timeout interval;  
    t_timeout timeout;
    timeout_get_timeout(&interval, sample_time);
    timeout_get_current_time(&timeout);

    // angle[0] = base, angle[1] = pendulum
    double angle[2] = { 0.0, 0.0 }; 
    double voltage = 0.0;
    int32_t encoder_counts[2];
    uint32_t encoder_channels[2] = { 0, 1 };
    uint32_t analog_channels[2] = { 0 }; 
    uint32_t digital_channels[1] = { 0 };
    t_boolean digital_values[1] = { 1 }; 
    hil_write_digital(board, digital_channels, 1, digital_values);

    // swing-up standing gate
    bool stand_run = false;
    double er = 0.1;

    // calculation angle
    double theta = 0.0;
    double alpha = 0.0;
    double alpha_deg = 0.0;

    // TCP/IP ready
    tcp_server tcsp = tcp_server(host, port);

    // control loop
    for (int i = 0; i < (int)((double)simulation_time / sample_time); i++) {
        timeout_add(&timeout, &timeout, &interval);
        qtimer_sleep(&timeout);

        // read sensor
        hil_read_encoder(board, encoder_channels, 2, encoder_counts);
        angle[0] = encoder_counts[0] * (2.0 * M_PI / 2048.0);
        angle[1] = encoder_counts[1] * (2.0 * M_PI / 2048.0);

        // calc output
        theta = -angle[0];
        alpha = fmod(angle[1], 2.0 * M_PI) - M_PI;
        alpha_deg = alpha * 180 / M_PI;

        if (!stand_run)
        {
            if (abs(alpha) < er)
            {
                cout << "set" << endl;
                stand_run = true;
                continue;
            }
        }
        else
        {
            tcsp.Send<string>("run");

            tcsp.Send<double>(-theta);
            tcsp.Send<double>(-alpha);

            voltage = tcsp.Recv<double>();

            cout << "---------------------------------------------" << endl;
            cout << "pendulum angle: " << alpha << endl;
            cout << "base angle: " << theta << endl;
            cout << "control input: " << voltage << endl;

            if (abs(voltage) > 15)
            {
                voltage = 0;
            }
        }


        // actuator write
        hil_write_analog(board, analog_channels, 1, &voltage);
    }

    tcsp.Send<string>("end");

    // logic terminate
    voltage = 0.0;
    hil_write_analog(board, analog_channels, 1, &voltage);
    digital_values[0] = 0;
    hil_write_digital(board, digital_channels, 1, digital_values);
    if (board != NULL)
    {   
        hil_close(board);
    }
    
    return 0;
}

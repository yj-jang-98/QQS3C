#include "tcp_protocol_server.h"
#include <string>
#include <iostream>
using namespace std;

const string host = "0.0.0.0";
const int port = 9999;

int main()
{
    tcp_server tcsp = tcp_server(host, port);

    tcsp.Send<int>(60207);

    int i = tcsp.Recv<int>();
    cout << "recv : " << i << endl;


    tcsp.Send<double>(60.207);

    double f = tcsp.Recv<double>();
    cout << "recv : " << f << endl;

    
    tcsp.Send<string>("60-207");

    string s = tcsp.Recv<string>();
    cout << "recv : " << s << endl;
}

# QQS3C
QQS3C provides the drive code for encrypted control for the Quanser Qube Servo 3 model. 
The code transforms dynamic controllers through various methods and then drives the system through homomorphic encryption. 
The cryptographic libraries for computational homomorphism use [Microsoft SEAL](https://github.com/microsoft/SEAL), [OpenFHE-python](https://github.com/openfheorg/openfhe-python), and [CDSL-EncryptedControl](https://github.com/CDSL-EncryptedControl/CDSL/tree/main) using [lattigo](https://github.com/tuneinsight/lattigo). The code uses Quanser's Qube Servo 3 model [Qube Servo 3](https://github.com/quanser/Quanser_Academic_Resources/tree/dev-windows) Python API. 

---

## Implementation Direction
The code was implemented through data communication with the Quanser API via TCP/IP in order to use Microsoft's SEAL, a C++ based homomorphic encryption library that can be operated, lattigo (CDSL) written in Go, and openFHE-python that can be run in a Linux environment, since the Quanser hardware API is provided only for Python and runs in a Windows environment.

---

## Features
The code implements controller versions in Python, C++, and Go.
The interfacing code for the Python simulator and the actual hardware, corresponding to each controller, can be found in the "interface/plant" directory.
The actual device consists of a single file, "plant.py" in "interface/plant/py/hardware", while the simulator consists of "model.py" and "plant.py" in "interface/plant/py/simulation".
**Code explanation and technical interpretation can be found at the link [QQS3C-obsidian](https://publish.obsidian.md/qqs3c)**

### Python version controller
You can check the "ctrl_*.py" controller file, which is written in Python, ll_state_feedbin the "interface/controller/py" folder of the code.
They are implemented in five technically different forms, which are named by state_filter, full_state_feedback, observer_form, arx_model, integer_matrix, respectively.
In each folder, both "model.py" and "model_enc.py" are files that implement objects for controller and encrypted control.

1. **state_filter**:Using d/dt filter:
   * ctrl_sf.py
     
      ↳ Using d/dt filter from Quanser Qube Servo 3. This code is **not available**.
2. **full_state_feedback**:Using observer for full state feedback:
   * ctrl_fs.py
     
     ↳ "ctrl_fs.py" is an observer-based design, but it is a code in which the observer runs in the plant and the controller operates in full state.
   * ctrl_fs_q.py
     
     ↳ "ctrl_fs_q.py" is a quantized version of "ctrl_fs.py".
   * ctrl_fs_enc.py
     
     ↳ "ctrl_fs_enc.py" is a BGV-type encrypted version of "ctrl_fs_q.py" with openFHE.
3. **observer_form**:Using observer:
   * ctrl_obs.py
     
     ↳ "ctrl_obs.py" is a code that uses observer-based controller design.
   * ctrl_obs_q.py
     
     ↳ "ctrl_obs_q.py" is a quantized version of "ctrl_obs.py".
   * ctrl_obs_enc.py
     
     ↳ "ctrl_obs_enc.py" is a BGV-type encrypted with re-encryption method of "ctrl_obs_q.py" with openFHE. This code is **not available**.
4. **arx_model**:Using ARX transformed from observer:
   * ctrl_arx.py
     
     ↳ "ctrl_arx.py" implements a controller converted from an observer-based design to an AutoRegressive & eXogenous input (ARX) model based on observability.
   * ctrl_arx_q.py
     
     ↳ "ctrl_arx_q.py" is quantized version of "ctrl_arx.py".
   * ctrl_arx_enc.py
     
     ↳ "ctrl_arx_enc.py" is a BGV-type encrypted with re-encryption method of "ctrl_arx_q.py" with openFHE.
5. **integer_matrix**:Using integer matrix transformed from observer:
   * ctrl_intmat.py
     
     ↳ "ctrl_intmat.py" is a code that converts the controller state matrix into an integer based on the observability of the observer-based controller.
   * ctrl_intmat_q.py
     
     ↳ "ctrl_intmat_q.py" is quantized version of "ctrl_intmat.py".

### C++ version controller
You can check the "ctrl_arx_enc.cpp" controller file, which is written in C++, in the "interface/controller/cpp/arx_model/" folder of the code.
In cpp, only the encrypted controller of "ctrl_arx_q.py" provided in Python is provided.
"model_enc.h" contains an object of the encrypted controller.

1. **arx_model**:Using ARX:
   * ctrl_arx_enc.cpp
     
     ↳ Unlike "ctrl_arx_enc.py" provided by Python, "ctrl_arx_enc.cpp" is encrypted using Microsoft SEAL. This allows for slightly faster sampling times.

### Go version controller
You can check the "ctrl_intmat_enc.go" controller file, which is written in Go, in the "interface/controller/go/integer_matrix/" folder of the code.
In go, only "ctrl_intmat_enc.go", which is an encrypted file of "ctrl_intmat_q.py" provided by Python, is provided.
"model_enc.go" contains a function of the encrypted controller.

1. **integer_matrix**:Using integer matrix:
   * ctrl_intmat_enc.go
     
     ↳ "ctrl_intmat_enc.go" is an encrypted version of "ctrl_intmat_q.py". (An equivalent encrypted controller was not provided in Python.)
    
--- 

## How to use
It explains the preparations before use, how to use the simulation file, how to use the Ouanser Interactive Labs, and how to use the actual hardware.

### Before using
This code should work for both Windows and WSL (Windows Subsystem for Linux) environments.(**If you want to use only Windows environment, then only can't use OpenFHE-python wrapper and needs more several setting**)
Please refer to the link [WSL installation method](https://learn.microsoft.com/ko-kr/windows/wsl/install) for instructions on installing WSL.

This requires three essential elements:

1. Go version 1.25.1 or later
2. C++ 17 compiler or later
3. Python 3.12 or later
   
at least. (The following description is after installing the above three elements)

If WSL is installed, the appropriate Linux OS is Ubuntu-24.04 LTS version. 

And next is Quanser requirement on Windows. You need to installation Quanser's api and library to use this code. 

### Settings for operation
There exist two way to use this library. One is using both Windows and WSL environment, The other is using only Windows environment.
This section introduce setting method of both side.

#### Using Windows and WSL
##### WSL environment
Assuming you have Python and Go installed.
If not, you should refer to the above version and install it.

1. Microsoft SEAL installation
   * See the "SEAL installation method.txt" file on the main page.
  
2. Essential Python package installation
   * First, download the relevant code via git clone on WSL bash page.
     ``` bash
       git clone "https://github.com/RFA0608/QQS3C.git"
     ```
   * Navigate to the downloaded directory.
     ``` bash
       cd QQS3C
     ```
   * Activate Python's virtual environment.
     ``` bash
       python3 -m venv venv
       source ./venv/bin/activate
     ```
   * Download all required packages using pip.
     ``` bash
       pip install numpy matplotlib control openfhe
     ```
3. Link complier and interpreter of communication tools
   * First, move directory to root folder.
     ``` bash
       cd QQS3C
     ```
   * Find absolute directory address and memorize this.
     ``` bash
       pwd
     ```
   * Change the address above to *** below. (C++ Link)
     ``` bash
       export CPATH=$CPATH:***/communication/cpp
     ```
   * And with same address, change to below.
     ``` bash
       pip install -e "***/communication/py"
     ```
4. Lattigo installation
   * This is automatically handled by go mod tidy, so no preparation is required.
     
##### Windows environment
1. You need to download the code via git clone on PowerShell page.
   ``` powershell
     git clone "https://github.com/RFA0608/QQS3C.git"
   ```
2. Execute the following task in Windows PowerShell.
   * Navigate to the downloaded directory.
     ``` powershell
       cd QQS3C
     ```
   * Activate Python's virtual environment.
     ``` powershell
       py -3 -m venv venv
       .\venv\Scripts\Activate.ps1
     ```
     or
     ``` powershell
       python3 -m venv venv
       .\venv\Scripts\Activate.ps1
     ```
     (If the above doesn't work, try the one below.)
     
     If the command doesn't work, try again by following these steps:
       1. Launch PowerShell as administrator.
       2. Set execution policy
          ``` powershell
            Set-ExecutionPolicy RemoteSigned
          ```
       3. Turn off the administrator PowerShell, open a standard (non-administrator) PowerShell, and try the command again.
   * Download all required packages using pip.
     ``` powershell
       pip install numpy matplotlib control openfhe PyQt6 pyqtgraph
     ```
3. Link interpreter
   * First, move directory to root folder.
     ``` powershell
       cd QQS3C
     ```
   * Find absolute directory address and memorize this.
     ``` powershell
       pwd
     ```
   * Change the address above to *** below.
     ``` powershell
       pip install -e "***/communication/py"
     ```
   * **In OPTION 1's step 7, you have to do.**
5. You need to check the hyper-v ip for TCP/IP communication between the Windows and WSL.
   ``` powershell
     ipconfig
   ```
   Save IPv4 address of vEthernet (WSL (Hyper-V...)).
   
**OPTION 1** if you want to use Ouanser Interactive Labs(QLab), additionally follows below.
1. Enter the url [portal_quanser](https://portal.quanser.com/Downloads), find 'these instructions' in "For Python users" section, and find 'Get Started' in "Design Philosophy" section.
2. Download and install Quanser Interactive Labs to click 'Windows' in "Attention" section.
3. Download and install SDK to click 'Download Quanser SDK for Windows' in "Attention Windows" section.
4. If you do not touch any option during installing, you can find 'quanser_api' word in "Program Files/Quanser/Quanser SDK/python" path. Just check this file.
5. Enter the url [quanser](https://github.com/quanser/Quanser_Academic_Resources), download library(whole things) and unzip proper path(like document).
6. In the QQS3C "interaction/plant/py/hardware", the top side, "sys.path.append(r"-")" change the path "-" to the path set in step 5.
7. Make sure venv is on, write (**VERY IMPORTANT: Anything on Quanser requires this, so it must be installed**)
  ``` powershell
    python -m pip install --upgrade --find-links "C:\Program Files\Quanser\Quanser SDK\python" "C:\Program Files\Quanser\Quanser SDK\python\quanser_api-2025.11.1-py2.py3-none-any.whl"
  ```
  on the terminal and connect the SDK (this path can find in step 4).

**OPTION 2** If you want to use QUARC-C based plant code (more suitable real-time interaction than python), need more setting on Visual Studio(VS 2022)
1. Install Visual Studio and QLab(according to the above content).
2. Make a new project.
3. Put the file plant.cpp, which is located in "interface/plant/cpp/hardware", to source file section.
4. Put the file tcp_protocol_server_windows.h, which is located in "communication/cpp", to header file section.
5. Enter project configuration, that is located project->Properties, Change C++ Language Standard C++17 (maybe it was C++14)
6. Find the address of "Quanser SDK/include" and paste on C/C++->Additional Include Directories section. (maybe there's a Quanser SDK in the Quanser folder in Program Files, or there's a QUARC in it)
7. Find the address of "Quanser SDK/lib/win64" and paste on Linker->General->Additional Library Directories section.
8. Move to Linker->Input->Additional Dependencies section, put 'hil.lib', 'quanser_runtime.lib', 'quanser_common.lib' in their.

#### Using only Windows
##### Windows environment
This is exactly the same as the Windows setting in the WSL and Windows description, and of course, the OPTION part. But I can't use OpenFHE-python wrapper here.

**OPTION 3** If you want to use "communication/cpp" in Windows
1. Open Visual Studio.
2. Put the file tcp_protocol_client_windows.h, which is located in "communication/cpp", to header file section.

### Ready to operate
There are two different executions in each environment.

#### WSL environment
1. Go to the previously downloaded QQS3C folder location and run the debugger (vscode) to write below.
   ``` bash
     code .
   ```
2. Here, each file provided in three languages (py, cpp, go) has a different execution method.
   * Python
     1. Find controller description code set which are located in "interface/controller/py" folder on debugger (vscode).
     2. Select the controller file you want to run.
     3. In that file, change 'localhost' in HOST variable to the vEthernet ip you saved earlier.
     4. Get ready to press F5 button.
   * C++
     1. Find controller description code set which are located in "interface/controller/cpp/arx_model" folder on debugger (vscode).
     2. Select the controller file you want to run.
     3. In that file, change 'localhost' in HOST variable to the vEthernet ip you saved earlier.
     4. In the bash window, move directory to "interface/controller/cpp/arx_model".
        ``` bash
          cd interface/controller/cpp/arx_model
        ```
     5. Create a new make file using cmake.
        ``` bash
          cmake .
        ```
     6. Create an executable binary file using the make file.
        ``` bash
          make
        ```
     7. If you see a file called "ctrl_arx_enc" then you are done and ready to write the following in the bash window and press enter
        ``` bash
          ./ctrl_arx_enc
        ```
   * Go
     1. Find controller description code set which are located in "interface/controller/go" folder on debugger (vscode).
     2. Select the controller file you want to run.
     3. In that file, change 'localhost' in HOST variable to the vEthernet ip you saved earlier.
     4. In the bash window, move directory to "interface/controller/go/integer_matrix".
        ``` bash
          cd interface/controller/go/integer_matrix
        ```
     5. Set GOPATH to the current directory.
        ``` bash
          pwd
        ```
        Copy the result and paste it into *** below.
        ``` bash
          export GOPATH=***
        ```
     6. At that location, write something like the following and be ready to press enter.
        ``` bash
          go run .
        ```
This completes the controller's preparation for operation.

#### Windows environment
1. Go to the previously downloaded QQS3C folder location and run the debugger (vscode) to write below.
    ``` powershell
       code .
    ```
2. Here, it is divided depending on whether simulation is performed or actual hardware is operated.
   * Simulation
     1. Find plant description code set which are located in "interface/plant/py/simulation" folder on debugger (vscode).
     2. Select the controller file named "plant.py"
     3. Get ready to press F5 button.
   * Quanser Interactive Labs
     1. Lanch Quanser Interactive Labs before we installed, login, and select "Qube 3 -Pendulum".
     2. Find plant description code set which are located in "interface/plant/py/hardware" folder on debugger (vscode).
     3. Select the controller file named "plant.py" or "plant_with_swing_up.py".
     4. Find variable 'hardware' in "def control_loop()" and change value to 0.
     5. Get ready to press F5 button.
   * Hardware
     1. Find plant description code set which are located in "interface/plant/py/hardware" folder on debugger (vscode).
     2. Select the controller file named "plant.py" or "plant_with_swing_up.py".
     3. Get ready to press F5 button.
3. Additionaly, if you use only Windows environment.
   * Controller
     1. Choose controller code (Except for using OpenFHE-python).
     2. And ready to press F5 button.

This completes the plant's preparation for operation.

**Additional Guide** Method Using SEAL On Windows
1. Install vcpkg.
   ``` powershell
     git clone https://github.com/microsoft/vcpkg.git
     .\vcpkg\bootstrap-vcpkg.bat
   ```
2. Install SEAL.
   ``` powershell
     .\vcpkg\vcpkg install seal:x64-windows
     .\vcpkg\vcpkg integrate install
   ```
3. Onpen Visual Studio.
4. Change project type debug to release.
5. Enter project configuration, that is located project->Properties, Change C++ Language Standard C++17 (maybe it was C++14)
6. In section Configuration Properties, you can find vcpkg->Use Vcpkg/Install Vcpkg Dependencies and check type is Yes. (If are not, change Yes)
7. In section C/C++, you can find Preprocessor->Definitions and add `;NOMINMAX` at backside.

### Operation
Proceed in the following order.
1. Press F5, which is waiting for Windows environment.
2. Press F5 or enter to launch the controller that was waiting in WSL (or Windows).

If you ran a simulation, a graph of the output will appear in the file "plant output as sim.png" in "interface/plant/py/simulation/result" folder.

If you ran a Quanser Interactive Labs, you can see movement on QLabs.

If you are running real hardware, there is two side of launch, first is manually raise the pendulum(use "plant.py" code), second automatically swing up the pendulum(use "plant_with_swing_up.py"), so that control start is True while looking at the output of the debugger (vscode) running in the Windows environment.
One thing to note is that for it to work, both the pendulum and the base must be near the equilibrium point.

---

## Demonstration
1. QQS3C Installation Guide:
https://youtu.be/01qr6Mvyikw
(This YouTube link only supports Korean)
   
3. Quanser Interactive Labs Test:
https://youtu.be/ncy-5f4BtY0

4. Hardware Test:
https://youtu.be/kVwAEByurqQ
(This YouTube link only supports Korean)

The flow of the video is as follows(Hardware only).
1. "ctrl_fs_enc.py" for hardware demo
2. "ctrl_arx_enc.cpp" for hardware demo
3. "ctrl_intmat_enc.go" for hardware demo

In the video, each hardware demonstration is run for about 30 seconds to check whether control was possible.

> **[INFO] Security**
> 
> - "ctrl_fs_enc.py" does not satisfy 128-bit lambda security.
> 
> - "ctrl_intmat_enc.go" is also like that.
> 
> - On the other hand, "ctrl_arx_enc.cpp" sufficiently satisfies 128-bit lambda security.

# Contact
If you need help or explanation while using this library, please send me an email below and I will respond.

- jeongmingyu@cdslst.kr (Mingyu Jeong)
- leesangwon@cdslst.kr (Sangwon Lee)
- leedonghyun@cdslst.kr (Donghyun Lee)

Provided by SEOULTECH CDSL.

# Licenses & Acknowledgements
This project utilizes code from several open-source projects. We express our gratitude to their developers. The licenses for these dependencies are listed below.

* **Lattigo (v6)**
    * Licensed under the [Apache License 2.0](https://github.com/tuneinsight/lattigo/blob/main/LICENSE)

* **Microsoft SEAL**
    * Licensed under the [MIT License](https://github.com/microsoft/SEAL/blob/main/LICENSE)

* **CDSL-EncryptedControl**
    * Licensed under the [MIT License](https://github.com/CDSL-EncryptedControl/CDSL/blob/main/LICENSE)

* **OpenFHE (Python)**
    * Licensed under the [BSD 2-Clause License](https://github.com/openfheorg/openfhe-python/blob/main/LICENSE)

* **Numpy**
    * Licensed under the [BSD 3-Clause License](https://github.com/numpy/numpy/blob/main/LICENSE.txt)

* **Matplotlib**
    * Licensed under the [PSF-style License](https://github.com/matplotlib/matplotlib/blob/main/LICENSE/LICENSE)

* **Python Control Systems Library (python-control)**
    * Licensed under the [BSD 3-Clause License](https://github.com/python-control/python-control/blob/main/LICENSE)

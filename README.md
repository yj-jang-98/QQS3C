# QQS3C
QQS3C provides the drive code for encrypted control for the Quanser Qube Servo 3 model. 
The code transforms dynamic controllers through various methods and then drives the system through homomorphic encryption. 
The cryptographic libraries for computational homomorphism use [Microsoft SEAL](https://github.com/microsoft/SEAL), [OpenFHE-python](https://github.com/openfheorg/openfhe-python), and [CDSL-EncryptedControl](https://github.com/CDSL-EncryptedControl/CDSL/tree/main) using [lattigo](https://github.com/tuneinsight/lattigo). The code uses Quanser's Qube Servo 3 model [Qube Servo 3](https://github.com/quanser/Quanser_Academic_Resources/tree/dev-windows) Python API. 

---

## Implementation direction
The code was implemented through data communication with the Quanser API via TCP/IP in order to use Microsoft's SEAL, a C++ based homomorphic encryption library that can be operated, lattigo (CDSL) written in Go, and openFHE-python that can be run in a Linux environment, since the Quanser hardware API is provided only for Python and runs in a Windows environment. (Windows-only version also works)

``` mermaid
graph LR
    subgraph Windows_Host [Windows Host: Plant]
        A[Quanser Hardware / QLab] <--> B[Python API / C++ SDK]
        B <--> C{TCP Server}
    end

    subgraph WSL_Guest [WSL / Remote: Controller]
        D{TCP Client} <--> E[Encrypted Controller]
        E --- F[SEAL / OpenFHE / Lattigo]
    end

    C <--> |TCP/IP| D
```

---

## Features
The code implements controller versions in Python, C++, and Go.
The interfacing code for the Python simulator and the actual hardware, corresponding to each controller, can be found in the "interface/plant" directory.
The actual device consists of a single file, "plant.py" in "interface/plant/py/hardware", while the simulator consists of "model.py" and "plant.py" in "interface/plant/py/simulation".
**Code explanation and technical interpretation can be found at the link [QQS3C-obsidian](https://publish.obsidian.md/qqs3c)**

### Controller description
You can check the "ctrl_*.py" controller file, which is written in Python, in the "interface/controller/py" folder of the code.
They are implemented in five technically different forms, which are named by state_filter, full_state_feedback, observer_form, arx_model, integer_matrix, respectively.
In each folder, both "model.py" and "model_enc.py" are files that implement objects for controller and encrypted control.
There are also C++ and Go versions of encrypted controllers for faster and more appropriate cryptographic techniques. 
you can find "interface/controller/C++/arx_model" and "interface/controller/go/integer_matrix".
They are in order a version implemented in Python as Microsoft SEAL C++ by arx_model and a version implemented in Lattigo (CDSL) by integer_matrix.


**Controller Compatibility**

| Model | Language | Encryption | Security (128-bit) | Status | Python Series | Other Series |
| :--- | :---: | :---: | :---: | :--- | :--- | :--- |
| **state_filter(d/dt filter)** | Python | - | - | **Not Available** | nominal | Ⅹ | 
| **full_state_feedback** | Python | BGV (OpenFHE-python) | △ | Available | nominal, quantized(_q), encrpyted(_enc) | Ⅹ |
| **observer_form** | Python | BGV (OpenFHE-python) | Ⅹ | **Not Available** | nominal, quantized(_q) | Ⅹ | 
| **arx_model** | Python/C++ | BGV (OpenFHE-python/SEAL) | ◎ | Available | nominal, quantized(_q), encrypted(_enc) | encrypted(_enc) |
| **integer_matrix** | Python/Go | RGSW (CDSL lattigo) | △ | Available | nominal, quantized(_q), encrypted(_enc) | encrypted(_enc) |

Note: Nominal refers to the controller as designed, '_q' is the quantized version of the state variable state matrices, and '_enc' is the encrypted version.

--- 

## How to use
It explains the preparations before use, how to use the simulation file, how to use the Ouanser Interactive Labs, and how to use the actual hardware.

### Before using
This project supports both Windows and WSL environments. (Note: The OpenFHE-python wrapper is unavailable in a Windows-only setup, and additional configuration is required)
Please refer to the link [WSL installation method](https://learn.microsoft.com/ko-kr/windows/wsl/install) for instructions on installing WSL.

This requires three essential elements:

1. Go version 1.25.1 or later
2. C++ 17 compiler or later
3. Python 3.12 or later
   
at least. (The following description is after installing the above three elements)

If WSL is installed, the appropriate Linux OS is Ubuntu-24.04 LTS version. 

You can find Installation guide in [**QQS3C-obsidian > Installation guide**](https://publish.obsidian.md/qqs3c/%EB%9D%BC%EC%9D%B4%EB%B8%8C%EB%9F%AC%EB%A6%AC/QQS3C/Introduction/Installation+guide?). The installation method is **quite tricky**, so please refer to it.
1. Only Windows users refer to
   [QQS3C-obsidian > Using Windows only](https://publish.obsidian.md/qqs3c/%EB%9D%BC%EC%9D%B4%EB%B8%8C%EB%9F%AC%EB%A6%AC/QQS3C/Introduction/Using+Windows+only?)
2. Both Windows and WSL users refer to
   [QQS3C-obsidian > Using both Windows and WSL](https://publish.obsidian.md/qqs3c/%EB%9D%BC%EC%9D%B4%EB%B8%8C%EB%9F%AC%EB%A6%AC/QQS3C/Introduction/Using+both+Windows+and+WSL?) 

---

# Demonstration
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

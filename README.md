# Generic CDI Plugin

Written to use gpu pods with k3s on NixOS.

[This PR](https://github.com/NixOS/nixpkgs/pull/284507) implemented CDI for NVIDIA GPUs.
This plugin can be installed in kubernetes to enable gpu pods.

## Usage

Create a daemonset for the plugin
```
apiVersion: v1
kind: Namespace
metadata:
  name: generic-cdi-plugin
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: generic-cdi-plugin-daemonset
  namespace: generic-cdi-plugin
spec:
  selector:
    matchLabels:
      name: generic-cdi-plugin
  template:
    metadata:
      labels:
        name: generic-cdi-plugin
        app.kubernetes.io/component: generic-cdi-plugin
        app.kubernetes.io/name: generic-cdi-plugin
    spec:
      containers:
      - image: ghcr.io/olfillasodikno/generic-cdi-plugin:main
        name: generic-cdi-plugin
        command: 
          - /generic-cdi-plugin
          - /var/run/cdi/nvidia-container-toolkit.json
        imagePullPolicy: Always
        securityContext:
          privileged: true
        tty: true
        volumeMounts:
        - name: kubelet
          mountPath: /var/lib/kubelet
        - name: nvidia-container-toolkit
          mountPath: /var/run/cdi/nvidia-container-toolkit.json
      volumes:
      - name: kubelet
        hostPath:
          path: /var/lib/kubelet
      - name: nvidia-container-toolkit
        hostPath:
          path: /var/run/cdi/nvidia-container-toolkit.json
      affinity:
        nodeAffinity:
          requiredDuringSchedulingIgnoredDuringExecution:
            nodeSelectorTerms:
            - matchExpressions:
              - key: "nixos-nvidia-cdi"
                operator: In
                values:
                - "enabled"
```

create NixOS configuration to enable cdi in containerd
```
services.k3s = {
  role = "server";
  clusterInit = true;

  extraFlags = (toString [
    "--container-runtime-endpoint unix:///run/containerd/containerd.sock"
  ]);

  label = {
    "nixos-nvidia-cdi" = "enabled";
  };
};
hardware.nvidia-container-toolkit.enable = true;
virtualisation.containerd = {
  enable = true;
  settings = {
      plugins."io.containerd.grpc.v1.cri" = {
        enable_cdi = true;
        cdi_spec_dirs = [ "/var/run/cdi" ];
      };
    };
};
```

Create gpu pod
```
apiVersion: v1
kind: Pod
metadata:
  name: hashcat-benchmark-pod
spec:
  containers:
  - name: hashcat-benchmark-container
    image: dizcza/docker-hashcat
    command: ["hashcat", "-d", "2" , "-b"]
    resources:
      requests:
        nvidia.com/gpu-all: "1"
      limits:
        nvidia.com/gpu-all: "1"
```

The container should output somethign similar to this:
```
hashcat (v6.2.6) starting in benchmark mode

Benchmarking uses hand-optimized kernel code by default.
You can use it in your cracking session by setting the -O option.
Note: Using optimized kernel code limits the maximum supported password length.
To disable the optimized kernel code in benchmark mode, use the -w option.

CUDA API (CUDA 12.4)
====================
* Device #1: NVIDIA GeForce RTX 3060, skipped

OpenCL API (OpenCL 3.0 CUDA 12.4.131) - Platform #1 [NVIDIA Corporation]
========================================================================
* Device #2: NVIDIA GeForce RTX 3060, 11904/12037 MB (3009 MB allocatable), 28MCU

Benchmark relevant options:
===========================
* --backend-devices=2
* --optimized-kernel-enable

-------------------
* Hash-Mode 0 (MD5)
-------------------

Speed.#2.........: 24545.3 MH/s (38.12ms) @ Accel:128 Loops:1024 Thr:256 Vec:8

```


NVIDIA smi on the host should now display the hashcat process
```
$ nvidia-smi
+-----------------------------------------------------------------------------------------+
| NVIDIA-SMI 550.78                 Driver Version: 550.78         CUDA Version: 12.4     |
|-----------------------------------------+------------------------+----------------------+
| GPU  Name                 Persistence-M | Bus-Id          Disp.A | Volatile Uncorr. ECC |
| Fan  Temp   Perf          Pwr:Usage/Cap |           Memory-Usage | GPU-Util  Compute M. |
|                                         |                        |               MIG M. |
|=========================================+========================+======================|
|   0  NVIDIA GeForce RTX 3060        Off |   00000000:07:00.0 Off |                  N/A |
| 32%   42C    P2            132W /  170W |    2992MiB /  12288MiB |    100%      Default |
|                                         |                        |                  N/A |
+-----------------------------------------+------------------------+----------------------+
                                                                                         
+-----------------------------------------------------------------------------------------+
| Processes:                                                                              |
|  GPU   GI   CI        PID   Type   Process name                              GPU Memory |
|        ID   ID                                                               Usage      |
|=========================================================================================|
|    0   N/A  N/A   1572566      C   hashcat                                      2986MiB |
+-----------------------------------------------------------------------------------------+
```

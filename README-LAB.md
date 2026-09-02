# Lab source for the patched vm-instance package

Exact content of the Cozystack v1.6.1 packages artifact
(sha256:2d7e07e4cf3d49a97f5b9c62ad732cfb93586483723788eed489b32643a1a8df)
restricted to apps/vm-instance, library/cozy-lib and system/vm-instance-rd,
plus the five files of cozystack/cozystack#3978 (hotpluggable disks, commit f3e4f10).

Consumed by a Flux GitRepository referenced from PackageSource
cozystack.vm-instance-application. Not a fork branch: orphan history.

Branch lab/v1.6.1-static-ip adds, on top of the above, networks[].ipAddress pinned through the
network backend the cluster serves (Kube-OVN per-network annotation, or the Cozyplane
sdn.cozystack.io/networks launcher annotation), with the regenerated schema and RD.

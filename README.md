# kubevirt2libvirt

`kubevirt2libvirt` is a command that exposes [KubeVirt]'s logic to translate
KubeVirt objects in YAML form into matching [libvirt] XML:

```yaml
$ cat examples/simple.yaml
apiVersion: kubevirt.io/v1
kind: VirtualMachine
spec:
  template:
    spec:
      domain:
        resources:
          requests:
            memory: 2Gi
```

```xml
$ bin/kubevirt2libvirt < examples/simple.yaml
<domain type="">
  <name>_</name>
  <memory unit="b">2147483648</memory>
  <os>
    <type></type>
  </os>
[...]
```

It currently supports `kubevirt.io/v1`'s `VirtualMachine` and
`VirtualMachineInstance` objects.

## Installing

```console
$ make install
```

## License

This project is released under the Apache 2.0 license. See [LICENSE](LICENSE).

[KubeVirt]: https://kubevirt.io/
[libvirt]: https://libvirt.org/

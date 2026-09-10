# wasip2 (retired as product)

Do not target stock wasip2 with flag packs.

The web-native machine is **wasigocvm**: [wasigocvm.md](wasigocvm.md),
[toolchain/README.md](../toolchain/README.md), `wasigocvm.bat`.

`compile.bat --wasip2` exits with a redirect. A wasip2 clang wrapper may
still be used under the hood until `toolchain/` has our own triple —
that is an implementation detail, not the ABI or the docs story.

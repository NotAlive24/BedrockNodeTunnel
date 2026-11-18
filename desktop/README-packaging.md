# Nethernet Desktop native packages

The desktop suite is packaged independently from the tunnel CLI. The CLI's
existing installers and release workflow remain unchanged.

## Linux

The Linux build uses GTK 4 and WebKitGTK 6.0. The release workflow produces:

- AppImage
- Debian/Ubuntu `.deb`
- Arch Linux `.pkg.tar.zst`

Debian and Arch packages declare the native GTK/WebKit runtime dependencies.
The AppImage is produced by Wails' AppImage packager, which bundles the
WebKit-related runtime components supported by that packager. Host graphics
and desktop integration libraries may still be required.

## Windows

The Windows build produces a per-user NSIS installer. It does not request
administrator privileges and uses the Wails WebView2 bootstrapper flow.
The Microsoft Edge WebView2 Runtime is required; the installer can offer its
bootstrapper when the runtime is missing.

## Tunnel engine

The desktop application intentionally installs and updates the matching
`nethernet-client` engine through its Settings screen. The engine is not
copied into system package directories by these installers, so engine updates
remain managed by the existing SHA-256-verified updater.

## Local packaging

Install Go 1.26+, Wails CLI v3.0.0, and the platform build dependencies, then
run `wails3 package` from this directory. Linux packaging additionally needs
the Wails AppImage tooling and nfpm package dependencies. Windows NSIS builds
need `makensis` available on PATH.

# webpenc

WebP encoding for uploaded pictures, in pure Go.

`libwebp.go` comes from [gen2brain/webp](https://github.com/gen2brain/webp)
v0.6.4 (libwebp v1.6.0 compiled to WebAssembly and translated to Go by
wasm2go). It's copied with only its package name changed. `encode.go` is our
own, written after that project's encoder.

It's copied rather than imported for two reasons:
- the upstream package loads the system's libwebp with dlopen at startup
  whenever one is installed (unless every build passes `-tags nodynamic`);
- it registers itself as the program-wide WebP decoder.

HotDog CMS decodes uploads with Go's own decoders and uses libwebp only to
encode pixels those decoders produced.

Licenses: `LICENSE` (gen2brain/webp, MIT) and `LICENSE.libwebp` (libwebp,
BSD-3-Clause).

To update: take `libwebp.go` from a newer gen2brain/webp release, change its
first `package webp` line to `package webpenc`, check that `Xencode` keeps its
signature (see `lib/webp.c` there), run `go test ./internal/media/...`, and
update the versions above.

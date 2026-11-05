#!/usr/bin/env sh
# Install libsignal-ffi's build tools on an Ubuntu CI runner. GitHub's
# hosted runners already have rustup; act's smaller image does not, so
# this falls back to Ubuntu's own rustup package. Never curl | sh.
set -eu

command -v rustup >/dev/null 2>&1 || { sudo apt-get update -q && sudo apt-get install -yq rustup; }
rustup toolchain install stable --profile minimal --no-self-update >/dev/null
sudo apt-get update -q
sudo apt-get install -yq cmake protobuf-compiler clang

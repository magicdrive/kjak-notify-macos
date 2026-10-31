#!/usr/bin/env bash
# verify-dist.sh DIST [VERSION]: check what Hephaestus left in DIST before
# it is uploaded. VERSION defaults to the one in the manifest.
#
#   - exactly two archives, kjak-notify-macos_<version>_darwin_{amd64,arm64}
#     .tar.gz, and kjak-notify-macos_checksums.txt listing them, and nothing
#     else at the top of DIST but the manifest, bin/ and homebrew/;
#   - the checksums match (sha256sum -c / shasum -a 256 -c);
#   - each archive holds exactly kjak-feather-notify-macos (mode 0755),
#     LICENSE and README.md (0644), regular files at its root;
#   - the executable is a Mach-O for the archive's architecture (file(1),
#     when installed).
# Runs on macOS and Linux (bash 3.2, BSD or GNU tools).
set -euo pipefail
export LC_ALL=C

dist="${1:?usage: verify-dist.sh DIST [VERSION]}"
project=kjak-notify-macos
exe=kjak-feather-notify-macos

fail() { echo "::error::verify-dist: $*" >&2; exit 1; }

[[ -f "$dist/hephaestus-manifest.json" ]] || fail "no manifest in $dist"
version="${2:-$(sed -n 's/^ *"version": *"\([^"]*\)".*/\1/p' "$dist/hephaestus-manifest.json" | head -1)}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$ ]] || fail "version '$version'"
checksums="${project}_checksums.txt"

expected="$(printf '%s\n' bin hephaestus-manifest.json homebrew "$checksums" \
  "${project}_${version}_darwin_amd64.tar.gz" "${project}_${version}_darwin_arm64.tar.gz" | sort)"
# homebrew/ exists only once the formula stage ran
actual="$(cd "$dist" && for f in * .*; do
  case "$f" in .|..|'.*'|'*') ;; *) printf '%s\n' "$f" ;; esac
done; echo homebrew)"
actual="$(sort -u <<<"$actual")"
[[ "$actual" == "$expected" ]] || fail "unexpected files in $dist:
$actual"

listed="$(awk '{print $2}' "$dist/$checksums")"
[[ "$listed" == "$(printf '%s\n' "${project}_${version}_darwin_amd64.tar.gz" "${project}_${version}_darwin_arm64.tar.gz")" ]] ||
  fail "$checksums lists: $listed"
if command -v sha256sum >/dev/null; then
  (cd "$dist" && sha256sum -c "$checksums")
else
  (cd "$dist" && shasum -a 256 -c "$checksums")
fi

for arch in amd64 arm64; do
  archive="$dist/${project}_${version}_darwin_${arch}.tar.gz"
  names="$(tar -tzf "$archive" | sort | tr '\n' ' ')"
  [[ "$names" == "LICENSE README.md $exe " ]] || fail "$archive holds: $names"
  verbose="$(tar -tvzf "$archive")"
  grep -Eq "^-rwxr-xr-x .* $exe\$" <<<"$verbose" || fail "$exe in $archive is not a 0755 file"
  grep -Eq '^-rw-r--r-- .* LICENSE$' <<<"$verbose" || fail "LICENSE in $archive is not a 0644 file"
  grep -Eq '^-rw-r--r-- .* README.md$' <<<"$verbose" || fail "README.md in $archive is not a 0644 file"
  if command -v file >/dev/null; then
    want=x86_64; [[ "$arch" == arm64 ]] && want=arm64
    tmp="$(mktemp -d)"
    tar -xzf "$archive" -C "$tmp" "$exe"
    kind="$(file -b "$tmp/$exe")"
    rm -rf "$tmp"
    [[ "$kind" == "Mach-O 64-bit "*"$want"* ]] || fail "$exe for $arch is: $kind"
    echo "$arch: $kind"
  fi
done
echo "dist OK: $project $version"

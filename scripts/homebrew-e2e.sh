#!/usr/bin/env bash
# Homebrew end-to-end test of the release formula, on this Mac.
#
# Builds the release archives with Hephaestus for a tag (v0.1.0 unless TAG
# is set), points a copy of the generated formula at the local archives
# (file:// URLs, same sha256), then runs the real Homebrew in a throw-away
# tap: brew audit --strict, brew style, brew install (terminal-notifier comes
# as its dependency), brew test, and checks:
#   - the installed executable is kjak-feather-notify-macos, and the formula
#     depends on terminal-notifier;
#   - the installed plugin finds and accepts the dependency, by the setting
#     and on PATH, under `brew --prefix` (/usr/local or /opt/homebrew),
#     without posting a notification (an empty body stops it after the
#     check);
#   - kjak (KJAK_BIN, v1.1.0) and the plugin installed by Homebrew: what
#     `kjak feather plugins` says of it, and kjak with the copy the caveats
#     describe (config validate, send --dry-run: nothing is posted).
#
# It installs into the machine's Homebrew prefix and removes the formula, the
# tap and its trust on exit; terminal-notifier stays installed if it was.
set -euo pipefail
export LC_ALL=C

repo="$(cd "$(dirname "$0")/.." && pwd)"
heph="${HEPHAESTUS:-hephaestus}"
kjak="${KJAK_BIN:-$repo/build/kjak}"
tag="${TAG:-v0.1.0}"
version="${tag#v}"
formula="kjak-notify-macos"
exe="kjak-feather-notify-macos"
tap="kjak-notify-e2e/t$RANDOM$RANDOM"
work="$(mktemp -d)"
# kjak checks every directory above a plugin: keep the work tree under a
# directory only this user can write to.
chmod 700 "$work"

export HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_DEVELOPER=1 HOMEBREW_NO_ASK=1 \
  HOMEBREW_NO_INSTALL_CLEANUP=1 HOMEBREW_NO_INSTALLED_DEPENDENTS_CHECK=1 \
  HOMEBREW_NO_ANALYTICS=1 HOMEBREW_NO_ENV_HINTS=1

trusts=false
brew trust --help >/dev/null 2>&1 && trusts=true

cleanup() {
  set +e
  brew uninstall --force "$tap/$formula" >/dev/null 2>&1
  if [[ "$trusts" == true ]]; then brew untrust --tap "$tap" >/dev/null 2>&1; fi
  brew untap --force "$tap" >/dev/null 2>&1
  rm -rf "$work"
}
trap cleanup EXIT

step() { printf '\n==> %s\n' "$*"; }
fail() { echo "FAIL: $*" >&2; exit 1; }

[[ "$(uname -s)" == Darwin ]] || fail "macOS only"
[[ -x "$kjak" ]] || fail "no kjak at $kjak (make integration builds it)"
prefix="$(brew --prefix)"
arch="$(uname -m)"; [[ "$arch" == x86_64 ]] && arch=amd64
echo "Homebrew $(brew --version | head -1), prefix $prefix, $arch; $("$heph" version); $("$kjak" version | head -1)"

step "Hephaestus: build, archive, checksum, formula for $tag"
cd "$repo"
"$heph" run --stages build,archive,checksum,formula --tag "$tag" --fresh
generated="dist/homebrew/$formula.rb"
grep -qx '    depends_on "terminal-notifier"' "$generated" || fail "the formula does not depend on terminal-notifier"
grep -qx '  depends_on :macos' "$generated" || fail "the formula is not macOS only"

step "Throw-away tap $tap"
brew tap-new --no-git "$tap" >/dev/null
if [[ "$trusts" == true ]]; then brew trust --tap "$tap"; fi
tapdir="$(brew --repository "$tap")"

step "The release formula as generated: brew audit --strict, brew style, its version"
cp "$generated" "$tapdir/Formula/$formula.rb"
brew audit --strict --formula "$tap/$formula"
brew style "$tap"
# shellcheck disable=SC2016 # Ruby code
got="$(brew info --json=v2 "$tap/$formula" | ruby -rjson -e 'f = JSON.parse($stdin.read)["formulae"][0]; puts f["versions"]["stable"], f["dependencies"].join(",")' | paste -sd' ' -)"
[[ "$got" == "$version terminal-notifier" ]] || fail "version and dependencies: $got"

step "The same formula on the local archives"
# Homebrew takes the version of the release URLs from their tag; a file://
# URL has none, so the copy states it.
sed -e "s#https://github.com/magicdrive/kjak-notify-macos/releases/download/$tag/#file://$repo/dist/#" \
  -e "s#^  license \"MIT\"\$#  version \"$version\"\\
&#" \
  "$generated" > "$tapdir/Formula/$formula.rb"
grep -qx "  version \"$version\"" "$tapdir/Formula/$formula.rb" || fail "no version in the local formula"
grep -c 'file://' "$tapdir/Formula/$formula.rb" | grep -qx 2 || fail "expected 2 local URLs"

brew audit --strict --formula "$tap/$formula"

step "brew install, brew test"
brew install --formula "$tap/$formula"
brew test "$tap/$formula"
[[ "$(brew deps --formula "$tap/$formula")" == terminal-notifier ]] || fail "brew deps: $(brew deps --formula "$tap/$formula")"
keg="$(brew --prefix "$tap/$formula")"
[[ "$(cd "$keg/bin" && ls)" == "$exe" ]] || fail "installed: $(ls "$keg/bin")"
[[ -x "$prefix/bin/$exe" ]] || fail "no $prefix/bin/$exe"
[[ "$(brew list --versions "$tap/$formula")" == "$formula $version" ]] || fail "installed $(brew list --versions "$tap/$formula")"

probe() { # setting-or-empty PATH
  local req='{"protocol_version":1,"message":{"body":""}}'
  [[ -n "$1" ]] && req="{\"protocol_version\":1,\"message\":{\"body\":\"\"},\"settings\":{\"terminal_notifier\":\"$1\"}}"
  env -i HOME="$HOME" PATH="$2" "$prefix/bin/$exe" <<<"$req"
}
step "The installed plugin finds terminal-notifier under $prefix (nothing is posted)"
for how in setting path; do
  if [[ "$how" == setting ]]; then out="$(probe "$prefix/bin/terminal-notifier" /usr/bin:/bin)"
  else out="$(probe "" "$prefix/bin:/usr/bin:/bin")"; fi
  echo "$how: $out"
  [[ "$out" == *'the message has no body to show'* ]] || fail "terminal-notifier by $how"
done

step "kjak and the plugin installed by Homebrew"
home="$work/home"; mkdir -p "$home/.config/kjak" "$home/bin"; chmod 700 "$home"
out="$(env -i HOME="$home" PATH="$prefix/bin:/usr/bin:/bin" "$kjak" feather plugins)"
echo "$out"
if grep -q '^notify-macos .* ok ' <<<"$out"; then
  echo "kjak accepts the plugin in the Cellar: update the caveats and README"
elif grep -q 'Cellar above it is writable by group or others' <<<"$out"; then
  echo "known: kjak v1.1.0 refuses it (Homebrew's Cellar is writable by group admin)"
else
  fail "unexpected kjak feather plugins output"
fi

step "kjak with the pinned copy the caveats describe (send --dry-run: nothing is posted)"
install -m 755 "$prefix/opt/$formula/bin/$exe" "$home/bin/"
cat > "$home/.config/kjak/config.toml" <<EOF
[feathers.mac]
type = "external"
plugin = "notify-macos"
path = "$home/bin/$exe"
settings = { terminal_notifier = "$prefix/bin/terminal-notifier" }
timeout = "10s"
EOF
run_kjak() { env -i HOME="$home" PATH="$home/bin:$prefix/bin:/usr/bin:/bin" "$kjak" "$@"; }
run_kjak feather plugins | tee "$work/plugins.txt"
grep -q "^notify-macos  $home/bin/$exe  *ok " "$work/plugins.txt" || fail "kjak refuses the copy"
run_kjak config validate
run_kjak send --to mac --dry-run "homebrew e2e"

step "brew uninstall"
brew uninstall "$tap/$formula"
[[ ! -e "$prefix/bin/$exe" ]] || fail "$prefix/bin/$exe is left"

echo
echo "Homebrew E2E passed for $tag on darwin/$arch ($prefix)"

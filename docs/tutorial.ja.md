# チュートリアル: kjak から macOS の通知を出す（実験的）

[English](tutorial.en.md) · [README](../README.md) · [制限事項](design/limitations.md)（英語）

## 1. 目的

`kjak-feather-notify-macos` を使うと、`kjak send` で macOS の通知を出せます。これは kjak の external feather 用プラグイン（プロトコル v1）で、terminal-notifier を実行します。kjak がプラグインにメッセージを渡し、プラグインがそれを terminal-notifier に渡し、terminal-notifier が macOS の通知センターに投稿します。

## 2. 実験的（Experimental）であること

- kjak 本体の一部ではなく、組み込みの feather でもありません。リリースも kjak とは別です。
- 実験的な間は、設定や表示するメッセージが変わることがあります。
- `delivered` は「macOS が通知を受け付けた」という意味です。表示されたことや、誰かが見たことまでは意味しません（12 章）。

## 3. 対応環境

| | |
|---|---|
| 対応 | macOS（ログイン中の GUI セッション）、kjak v1.1.0、terminal-notifier 3.1.0 |
| 実機で確認済み | terminal-notifier 3.1.0 単体（Intel（x86_64）の Mac、iTerm2 から）。このプラグインを kjak 経由で使う場合は未確認（9 章） |
| 未確認 | Apple silicon、ほかの macOS のバージョン、ほかの端末アプリ、SSH、launchd |

## 4. 必要なもの

- kjak v1.1.0（`kjak --version`）
- terminal-notifier 3.1.0（`terminal-notifier -version`）
- Go 1.27（プラグインのビルドに使います。`go version`）

## 5. terminal-notifier のインストール

```sh
brew install terminal-notifier
terminal-notifier -version        # terminal-notifier 3.1.0
command -v terminal-notifier      # /usr/local/bin（Intel）または /opt/homebrew/bin（Apple silicon）
```

Intel の Mac では、Homebrew（Intel の Mac は Tier 3 の扱いです）に bottle がなく、terminal-notifier をソースからビルドすることがあります。その場合は、正常に動く Xcode が必要です。ビルドが Xcode のフレームワーク（たとえば `DVTDownloads.framework`）の読み込みで失敗する場合は、Xcode の初回起動の処理を一度済ませてください。

```sh
sudo xcodebuild -runFirstLaunch
xcodebuild -checkFirstLaunchStatus; echo $?    # 0
```

これは Homebrew のサポートの水準の問題で、非対応という意味ではありません。terminal-notifier 3.1.0 は、Intel の Mac でビルドでき、通知を出せています。

## 6. プラグインのビルドとインストール

```sh
git clone https://github.com/magicdrive/kjak-notify-macos   # または手元のコピー
cd kjak-notify-macos
make install                      # ビルドして ~/.local/bin/kjak-feather-notify-macos に入れる
```

`make install PREFIX=/some/dir` とすると `/some/dir/bin` に入ります。そのディレクトリは絶対パスで `PATH` に含まれている必要があり、そのディレクトリとファイルはグループやほかの人が書き込めない状態でなければなりません。kjak は実行のたびにこれを確かめます。

```sh
kjak feather plugins              # notify-macos ... ok
```

## 7. kjak への登録

kjak には external feather を追加するコマンドがないので、`~/.config/kjak/config.toml` に書きます（`kjak config path` でファイルの場所がわかります）。

## 8. 設定例

```toml
# ~/.config/kjak/config.toml
[feathers.mac]
type = "external"
plugin = "notify-macos"                 # kjak-feather-notify-macos を実行する
settings = { terminal_notifier = "/usr/local/bin/terminal-notifier" }
timeout = "10s"
```

| キー | |
|---|---|
| `plugin` | `notify-macos`（`macos` だけの名前は kjak が予約しています） |
| `settings.terminal_notifier` | terminal-notifier の絶対パス（`command -v terminal-notifier` で確認）。Homebrew のリンクやスクリプト、または `terminal-notifier.app` の中の実行ファイルを指定できます。省略すると、`PATH` の絶対パスのディレクトリから最初に見つかった `terminal-notifier` を使います。実行するのは terminal-notifier 本体の実行ファイルだけで、Homebrew の Cellar、`/Applications`、`~/Applications` にあるものに限ります（ほかの場所に置いた場合は、そのディレクトリを `$TERMINAL_NOTIFIER_APP_DIRS` に書き、feather に `env = ["TERMINAL_NOTIFIER_APP_DIRS"]` を足します）。関わるファイルやディレクトリをほかのユーザーが変更できる場合は拒否します |
| `timeout` | kjak がプラグインに許す時間。2s 以上が必要で、10s（既定）を推奨します。プラグインはその 1 秒前に terminal-notifier を止め、`unknown` と報告します |
| `path`、`sha256` | 任意。プラグインのファイルを固定します（kjak のキーです） |

ほかの設定はありません。知らない設定は拒否します。秘密情報は不要です。複数の通知先にまとめて送るには、次のように書きます。

```toml
[groups.everywhere]
feathers = ["mac", "slack"]
```

確認します。

```sh
kjak config validate
kjak send --to mac --dry-run "test"     # プラグインを探して確かめるだけで、実行はしない
```

## 9. テスト通知

**次のコマンドは、画面に通知を出します。** まずプラグイン単体で、kjak が渡すのと同じ形のリクエストを送ります。

```sh
printf '%s' '{"protocol_version":1,"message":{"title":"kjak-notify-macos","body":"plugin test","level":"info","url":""},"settings":{"terminal_notifier":"/usr/local/bin/terminal-notifier"}}' \
  | kjak-feather-notify-macos; echo "exit $?"
```

期待する結果: 「kjak-notify-macos」「info」「plugin test」の通知が出て、stdout に `{"protocol_version":1,"status":"delivered","acceptance":"confirmed"}`、続いて `exit 0` が出ます。次に kjak 経由で送ります。

```sh
kjak send --to mac --title "kjak" --level success "Hello from kjak"
kjak send --to mac -o json "Hello again"        # results[0].status と .acceptance
```

期待する結果: `ok    mac (external) delivered ...` と表示されて終了コードが 0 になり、通知が出ます。表示されたかどうかは画面で確かめてください。kjak の終了コードがわかるのは、macOS が受け付けたかどうかまでです。

## 10. 日本語の通知

```sh
kjak send --to mac --title "ビルド完了 😀" --level success "日本語の本文
二行目もそのまま表示されます"
```

タイトル、level、本文は書いたとおりに通知に入ります。タイトルと level はそれぞれ 1 行になり、本文は改行がそのまま残ります。制御文字は空白に置き換えます。

## 11. 通知の許可

新しく入れた terminal-notifier が最初の通知を出すときは、許可を求めて答えを待ちます。その間に kjak の timeout が来て止めてしまうので、許可は一度だけ自分で済ませてください。

```sh
terminal-notifier -title "kjak-notify-macos" -message "permission test"
terminal-notifier -diagnose       # 許可の状態を表示する。通知は出さない
```

kjak が `notifications are not allowed for terminal-notifier (System Settings, Notifications): Notifications are turned off for this application.` と報告した場合は、**システム設定 → 通知 → terminal-notifier** で通知を許可してください（「通知を許可」、バナーまたは通知パネル）。

terminal-notifier は、macOS にもう一度許可を尋ねさせるために、許可をリセットする方法も案内します。

```sh
tccutil reset UserNotification fr.julienxx.oss.terminal-notifier
```

これは terminal-notifier に与えた許可を**消す**操作です（terminal-notifier を使うすべてのプログラムに影響します）。macOS にもう一度尋ねさせたいときだけ実行してください。プラグインのインストールでは実行しません。

## 12. 集中モードと応答不可

集中モード（応答不可）の間は、通知が画面に出ないことがあります。その場合でも、通常は通知センターに残ります。macOS は通知を受け付けているので、kjak は `delivered` と報告します。terminal-notifier の `-ignoreDnD` は使いません（Apple の entitlement が必要で、通常は効果がありません）。集中モード中にも表示したい場合は、集中モードの設定で terminal-notifier を許可してください。

## 13. トラブルシューティング

| kjak の表示 | 原因 | 対処 |
|---|---|---|
| `delivered` なのに画面に何も出ない | 集中モード、通知のスタイルが「なし」、画面のロック | 通知センターを見る。システム設定 → 通知 → terminal-notifier |
| `notifications are not allowed for terminal-notifier ...` | 通知が許可されていない | 11 章 |
| `terminal-notifier was not found in the absolute directories of PATH` | 未インストール、またはプラグインの `PATH` にない | `brew install terminal-notifier`、または `terminal_notifier` を設定する |
| `setting terminal_notifier must be an absolute path` | 相対パスを書いた | `command -v terminal-notifier` で絶対パスを調べる |
| `no terminal-notifier at ...` | 設定のパスにファイルがない | `command -v terminal-notifier` で調べる |
| `... is not terminal-notifier ...`、`must name a file called terminal-notifier` | 設定がほかのプログラムを指している | terminal-notifier のパスを書く |
| `... is writable by others` / `by its group` / `belongs to another user: it is not run` | インストールしたファイルかディレクトリを、ほかのユーザーが変更できる | `chmod go-w` する、または入れ直す |
| `too little time left to run terminal-notifier: give the feather a timeout of at least 2s` | `timeout` が 2s 未満 | `timeout = "10s"` にする |
| `... is not in a place terminal-notifier is installed to ...` | terminal-notifier.app がほかの場所にある | Homebrew で入れる、または `/Applications` に置く。あるいは `$TERMINAL_NOTIFIER_APP_DIRS` を使う（8 章） |
| `cannot check ...: no such file or directory` | 設定のパスのディレクトリが存在しない | `command -v terminal-notifier` で調べる |
| `the notification service did not answer (no GUI session?)`（acceptance unknown） | SSH、launchd、別のユーザー | デスクトップのセッションで kjak を実行する |
| `terminal-notifier did not finish before the deadline`（unknown） | 許可の応答待ち、システムが遅い | 許可を済ませる（11 章）。`timeout` を長くする |
| `unknown setting "..."` | このプラグインが知らない設定 | 設定は `terminal_notifier` だけ |
| `notify-macos runs on macOS only` | macOS 以外 | その環境ではほかの feather を使う |
| `kjak feather plugins` が notify-macos に問題（problem）を表示する | プラグインのファイルかディレクトリをほかの人が変更できる | 6 章 |
| `kjak feather plugins` に notify-macos が出ない（またはそのディレクトリが `skipped_path_elements` に出る） | そのディレクトリが `PATH` にない、または相対パスでしか入っていない | 絶対パスのディレクトリを `PATH` に入れる、または feather に `path` を書く |

`unknown` の結果は再試行しません。通知がすでに画面に出ているかもしれないためです。

## 14. アンインストール

```sh
make uninstall                    # ~/.local/bin/kjak-feather-notify-macos を削除する
```

設定から `[feathers.mac]`（と group の中の参照）を消してください。terminal-notifier は残しても、`brew uninstall terminal-notifier` で削除してもかまいません。terminal-notifier の通知の許可は、システム設定で削除するまで残ります。

## 15. 既知の制限

詳しくは[制限事項](design/limitations.md)（英語）を参照してください。

- macOS だけで動きます。GUI のセッションが必要です。
- 通知は terminal-notifier として表示されます（その名前とアイコン）。
- 音、アクション、クリック時の処理はありません。
- 再試行はしません。
- `delivered` は「見た」ことを意味しません。
- 実験的な機能です。

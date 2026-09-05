# リリース手順

対象は4つ。**成果物は生バイナリで、zip にはしない**(理由は下の「配布経路」)。

| 成果物 | 対象 | 備考 |
|---|---|---|
| `gpget-darwin-arm64` | macOS 15 以降 / Apple Silicon | cgo 必須 |
| `gpget-windows-amd64.exe` | Windows 10 以降 / x64 | ARM は x64 エミュレーションで動作 |
| `gpget-linux-amd64` | Linux / x86_64 | 完全静的 |
| `gpget-linux-arm64` | Linux / aarch64(Raspberry Pi 等) | 完全静的 |

Intel Mac、Windows 32bit、Windows ARM ネイティブは**対象外**。

---

## 0. 最初の1回だけ必要なこと

**このリポジトリにはまだ remote がありません。** GitHub にリポジトリを作って
push するところから始めます。

```bash
git remote -v      # 何も出なければ未設定
```

`gh` コマンドも**未インストール**です。Web の GitHub UI でも同じことはできますが、
以降の手順は `gh` 前提で書いてあります。

```bash
brew install gh
gh auth login
gh repo create gpget --public --source=. --remote=origin --push
```

**公開/非公開の判断**: 公開にするとリリースの URL が認証なしで叩けるので、
`curl` でのインストールがそのまま使えます。非公開だとテスターにも認証が要り、
`curl` の手順が破綻します。**テスターに配るなら公開が前提**です。

---

## 1. バージョンを決めてタグを打つ

**0.x の間は、コマンドの形と設定ファイルの項目が変わってよい。**gpget にとっての
「互換性」はライブラリ API ではなくそこなので、固まるまでは 0.x を維持する。

**プレリリース指定(`--prerelease`)は使わない。**GitHub の `releases/latest/` は
プレリリースを飛ばすため、README に書いた `latest/download/...` の
インストールコマンドが解決しなくなる。検証状況はリリースノートと README で伝える。

`v1.0.0` に上げる条件(現時点の想定):

- macOS / Windows / Linux の**3 OS すべて**で、手動転送と自動起動を実機確認できている
- コマンドと設定ファイルの形が落ち着いている


バージョン番号はタグとバイナリの両方に入ります。`gpget version` と
macOS の `.app` の `Info.plist` が同じ値を返すのは、ビルド時に
`-ldflags -X main.version` で埋めているためです。

```bash
git tag v0.1.0
```

**タグを打つ前に作業ツリーがクリーンであること。** 汚れていると
`git describe --dirty` が `-dirty` 付きの版を作ります。

---

## 2. ビルドする

```bash
./scripts/build-release.sh v0.1.0
```

**必ず macOS で実行すること。** darwin のビルドは cgo
(`UserNotifications` / `IOKit`)を使うため、他 OS からは作れません。
スクリプトは macOS 以外では最初に止まります。

スクリプトが自動で確認すること:

- **darwin バイナリが実際に両フレームワークをリンクしているか。**
  `GOARCH` を指定すると Go は `CGO_ENABLED` を 0 に落とすので、
  何もしないと**通知も USB 監視も効かない macOS 版**ができます。
  ビルドは成功し、起動もし、ユーザーには何も起きていないように見えます。
  この確認が唯一の歯止めです
- **`gpget version` が指定した版を返すか**(`-ldflags` の付け忘れ検出)
- **要求される macOS の最低バージョン**を表示する。これは
  ビルドマシンの SDK から自動的に決まる値で、フラグでは指定していません。
  **表示された値と README の対応バージョンがずれていないか毎回見ること**

出力は `dist/`(git 管理外)。`SHA256SUMS` も作られます。

---

## 3. 動作確認

最低限、**自分の Mac で**新しいバイナリを入れて一通り動かします。

```bash
cp dist/gpget-darwin-arm64 ~/bin/gpget       # または PATH の通った場所
gpget version
gpget probe                                   # カメラを繋いだ状態で
gpget status
gpget autostart install                       # "A test notification was sent" が出て、
                                              # 実際にバナーが出ることを目視確認
```

Windows / Linux は手元に環境が無ければここでは確認できません。
**テスターに渡す前提なら `docs/testing.md`(英語) / `docs/testing.ja.md`(日本語) を一緒に渡します。**

---

## 4. リリースを作る

```bash
git push origin main
git push origin v0.1.0

gh release create v0.1.0 \
  dist/gpget-darwin-arm64 \
  dist/gpget-windows-amd64.exe \
  dist/gpget-linux-amd64 \
  dist/gpget-linux-arm64 \
  dist/SHA256SUMS \
  --title "gpget v0.1.0" \
  --notes-file docs/release-notes/v0.1.0.md
```

リリースノートが無ければ `--notes "..."` で直接書いても構いません。

**バイナリを git にコミットしないこと。** 1本 9MB 前後 × 4本 × リリースごとに
履歴へ永久に残り、`git clone` が重くなります。消しても履歴からは消えません。
Release のアセットは git 履歴の外に置かれます。

---

## 5. 配布経路 — ここを間違えると Gatekeeper に止まります

macOS の `com.apple.quarantine` は**ダウンロードしたアプリが自分で付ける印**です。
Gatekeeper の「開発元を検証できません」は、**この印がある場合にだけ**出ます。

| 経路 | quarantine | 結果 |
|---|---|---|
| ブラウザで Release ページからダウンロード | **付く** | **止まる**(公証していないため) |
| `curl` / `wget` | 付かない | 通る |
| Homebrew(内部で curl) | 付かない | 通る |

**したがって、インストール手順は必ず `curl` のコマンドとして書くこと。**
「Releases ページからダウンロードしてください」と書くと、macOS では確実に
止まります。同じファイルでも、**案内の書き方で結果が変わります。**

なお `gpget.app` バンドルは `autostart install` がローカルで組み立てるので、
配布経路に関係なく quarantine が付きません。自動起動側にこの問題はありません。

Windows の SmartScreen(署名なし exe)がどう扱われるかは**未調査**です。
Linux には署名の問題自体ありません。

---

## 6. リリース後

- README のインストール手順のバージョン番号を更新する
- **Windows / Linux は自動起動が未検証**であることを README に明記したままにする。
  テスターから報告が来たら状態を更新する

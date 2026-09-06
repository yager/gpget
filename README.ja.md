# gpget

[English](README.md) | **日本語**

GoPro のメディアを、**microSD を抜かず・バッテリードアを開けず**に PC へ吸い出す CLI。
GoPro が公開しているローカル HTTP API を有線 USB で叩くだけ。

> **コマンドの出力は英語です。**このドキュメントだけが日本語です。

> gpget は独立したツールであり、GoPro 社とは無関係です(提携・出資・承認のいずれもありません)。
> "GoPro" は GoPro, Inc. の商標です。

---

## できること

- **増分オフロード** — カメラ内のメディアのうち、保存先にまだ無いものだけを撮影日ごとのフォルダへコピー
- **中断しても続きから** — Ctrl-C しても、ケーブルが抜けても、再実行すれば途中から再開する
- **チャプター分割 MP4 のリネーム** — 名前順で並べたときにバラけないファイル名へ(結合はしない)
- **バースト / インターバル / タイムラプスを 1 単位で扱う** — 数百〜数千枚をまとめてサブフォルダへ
- **カメラには一切書き込まない** — 完全リードオンリー。削除のコードを持たない

## 動作条件

| | |
|---|---|
| カメラ | **GoPro MISSION 1 PRO ILS** で検証。MISSION 1 系 / HERO11〜13 / MAX2 も `/gopro/media/list` を有線で公開していれば動く見込み |
| 接続 | **USB-C ケーブル**(データ転送対応のもの)。カメラを PC に挿すと有線ネットワークとして見える |
| OS | macOS / Windows / Linux |
| 備考 | **レンズや撮影モードは関係ない。**カメラが記録したものをそのまま吸い出す |

**microSD を抜く必要はありません。**メディアモッドを付けたままでも(側面 USB-C がデータを通せば)動きます。

---

## インストール

[Releases](https://github.com/yager/gpget/releases) からバイナリを1つ取るだけです。
ランタイムの導入は要りません。

> **ブラウザでダウンロードしないでください。**macOS では、ブラウザ経由で取得した
> ファイルに検疫属性が付き、署名していない gpget は Gatekeeper に止められます。
> 下の `curl` コマンドで取得すれば、この問題は起きません。

### macOS(Apple Silicon / macOS 15 以降)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-darwin-arm64
chmod +x ~/bin/gpget
echo 'export PATH="$HOME/bin:$PATH"' >> ~/.zshrc && source ~/.zshrc
gpget version
```

### Linux(x86_64 / aarch64)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
export PATH="$HOME/bin:$PATH"
gpget version
```

Raspberry Pi など aarch64 の場合は、URL の `gpget-linux-amd64` を
`gpget-linux-arm64` に変えてください(`uname -m` で確認できます)。

### Windows(64bit / Windows 10 以降)

PowerShell で:

```powershell
mkdir "$env:USERPROFILE\bin" -Force
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
$env:Path += ";$env:USERPROFILE\bin"
gpget version
```

`$env:Path` の変更はそのウィンドウだけ有効です。恒久的にするには、
システム環境変数の `Path` に `%USERPROFILE%\bin` を追加してください。

### ソースからビルドする場合

```bash
git clone https://github.com/yager/gpget && cd gpget
go build -o gpget .
```

Go 1.21 以降が必要です。macOS では cgo(通知と USB 監視)を使うため、
`CGO_ENABLED=1`(既定)のままビルドしてください。

### 対応環境

| OS | アーキテクチャ | 下限 |
|---|---|---|
| macOS | Apple Silicon | macOS 15 |
| Windows | x64 | Windows 10 |
| Linux | x86_64 / aarch64 | systemd(自動起動を使う場合) |

**Windows と Linux は自動起動を実機検証できていません。**手動の `sync` / `get` は
同じコードを通りますが、`autostart` は未検証です。試された方は
[Issues](https://github.com/yager/gpget/issues) で結果を教えていただけると助かります
(手順は [docs/testing.ja.md](docs/testing.ja.md))。

### アップデート

お使いの環境のバイナリをもう一度ダウンロードするだけです(その場で上書きされます)。
設定は触られません(場所は `gpget config path`)。

#### macOS

```bash
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-darwin-arm64
chmod +x ~/bin/gpget
gpget version
```

#### Linux

```bash
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
gpget version
```

aarch64 は `gpget-linux-amd64` を `gpget-linux-arm64` に変えてください。

#### Windows(PowerShell)

```powershell
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
gpget version
```

「使用中で上書きできない」と出たら、毎分動く自動起動タスクが `gpget.exe` を掴んでいます。
先に `gpget autostart uninstall` を実行してからダウンロードし直してください。

#### 自動起動を使っている場合

どの OS でも、アップデート後に install をやり直してください:

```
gpget autostart install
```

gpget は「接続時に実際に動くもの」の中に自分自身をコピーしているので、そのコピーを
作り直す必要があります。更新を自動検出して作り直す仕組みは入っていますが、確実なのは
`install` のやり直しだけで、置き場所や名前が変わる更新では必須です。
`gpget autostart status` は現在使われているコピーの場所を表示します。

`curl` の URL は常に最新リリースを取りに行くので、どこかの番号を書き換える必要はありません。

### アンインストール

```bash
gpget autostart uninstall   # 自動起動を有効にしていた場合のみ
rm ~/bin/gpget
```

`autostart uninstall` は LaunchAgent(Windows はタスク、Linux は systemd ユニット)と、
組み立てたアプリバンドルを消します。**設定と、取り込み済みのファイルは残ります。**
設定も消したい場合は、`gpget config path` が指すフォルダを手で削除してください。

| OS | 設定の場所 |
|---|---|
| macOS | `~/Library/Application Support/gpget/` |
| Windows | `%AppData%\gpget\` |
| Linux | `~/.config/gpget/` |

---

## 使い方

### まずこれだけ

```bash
gpget init     # 保存先などを対話で設定(初回のみ)
gpget sync     # カメラを繋いで実行。新しいメディアだけコピーされる
```

### コマンド一覧

```
gpget [sync]      日付フォルダへ増分オフロード(引数なし = sync)
    --dest <dir> --since <date> --date <date> --label <s>
    --type mp4,jpg --sidecars gpr,lrv --dry-run -y --ip <addr>

gpget list        ls -l 風の一覧(1 ファイル 1 行、グループは 1 行)
    --expand --new --date <date> --since <date> --video --photo --json --ip <addr>

gpget status      カード概要(日付別の件数・容量)+ 未オフロード量 + 孤児 .part

gpget get <name|glob>...   単一 / パターン指定のチェリーピック

gpget init        対話設定 → config.ini 生成
gpget config get <key> | set <key> <val> | path | edit

gpget autostart install | uninstall | status

gpget probe       接続診断
```

### `list` の出力イメージ

```
TYPE  DATE              SIZE   NAME                DEST
V     2026-09-04 14:32  2.4M   GX010014.MP4        -
P     2026-09-04 14:35  2.1M   GP010009.JPG(+GPR)  ✓
G     2026-09-03 08:00  20.1G  [interval ×1000]    GPAA0015…
```

1000 行超はそのまま出力する(`| less` / `grep` / `awk` で絞る想定)。ページャは持たない。

---

## 保存先とフォルダ名

保存パスは `<dest>/<folder>/<file>`、グループは `<dest>/<folder>/<group_dir>/<frame>`。

既定では撮影日ごとのフォルダに入る:

```
<動画ディレクトリ>/GoPro/          # mac ~/Movies, Win ~\Videos, Linux ~/Videos
├── 2026-09-04_GoPro/
│   ├── GX2495_01.MP4
│   ├── GX2495_02.MP4
│   ├── GP010009.JPG
│   ├── GP010009.GPR
│   └── GPAA0015/          ← バースト 20 枚
│       ├── GPAA0015.JPG
│       └── …
└── 2026-09-05_GoPro/
```

**日付フォルダはカメラが記録した撮影時刻(壁時計)そのまま。**転送後、ファイルの更新日時も撮影時刻に戻す。

### 保存先に使えない場所

安全のため、以下は**拒否**する:

- **クラウド同期フォルダ**(iCloud / Google Drive / Dropbox / OneDrive)
- **ネットワークボリューム**(SMB / NFS)
- 読み取り専用のドライブ
- 空き容量が足りないドライブ
- FAT32 / FAT16 で、単一ファイル上限を超えるファイルがある場合

大容量の動画を同期フォルダに置くと事故になりやすいため。外付け SSD などを推奨。

---

## 設定

### 場所

`os.UserConfigDir()` + `/gpget/config.ini`。保存先の既定も OS 固有(下記):

| OS | 設定ファイル | 既定の保存先 |
|---|---|---|
| macOS | `~/Library/Application Support/gpget/config.ini` | `~/Movies/GoPro` |
| Windows | `%AppData%\gpget\config.ini` | `%USERPROFILE%\Videos\GoPro` |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gpget/config.ini` | `$XDG_VIDEOS_DIR/GoPro`(既定 `~/Videos/GoPro`) |

`--config <path>` で上書き可。それ以外の設定ファイルは見ない(1 箇所に決め打ち)。
解決順:`--flag` > `config.ini` > 組み込み既定。`gpget config path` で実パスを表示。

### config.ini

```ini
[general]
dest       = /Users/you/Movies/GoPro    ; OS の動画フォルダ配下(gpget init が実パスを書く)
folder     = {date:%Y-%m-%d}_GoPro     ; 撮影日ごとのサブフォルダ
group_dir  = {stem}                    ; グループ用サブフォルダ(空 = 日付直下)
sidecars   = gpr,lrv                   ; none | gpr,lrv | all
overwrite  = skip                      ; skip | rename | replace
confirm    = true
timezone   = camera                   ; camera(変換なし) or +9 / -05:30(時計ズレ補正)

[chapters]
regroup      = multi                   ; always | multi | never
chapter_name = {prefix}{clip}_{chapter:02d}   ; → GX2495_01.MP4, GX2495_02.MP4 …

[camera]
ip =                                   ; 空 = 自動発見

[autostart]
mode = notify                          ; notify | auto
```

### テンプレート

`{token}` 置換。`{date:...}` は strftime 指定子。

- `folder` / `group_dir` で使える:`{date:%Y-%m-%d}` `{label}`(実行時 `--label`) `{model}` `{cam}` `{stem}`(グループ代表のファイル名幹)
- `chapter_name` で使える:`{prefix}`(`GX` 等) `{clip}`(末尾4桁) `{chapter:02d}` `{date:...}`
- 保存パス = `<dest>/<folder>/<file>`、グループは `<dest>/<folder>/<group_dir>/<frame>`
- **`{date:...}` の基準**:GoPro の `cre` はカメラの壁時計そのもの(UTC のフリで encode されている)。
  gpget は変換せずその年月日時分秒を使う。カメラの時計が現地時刻に合っていれば常に正しい。
  時計がズレていた/旅行先の時計だった場合だけ `timezone = +9` のようなオフセットで補正する

### `gpget init`

```
$ gpget init
Destination base directory                [<OS の動画フォルダ>/GoPro]:
Dated folder template                     [{date:%Y-%m-%d}_GoPro]:
Camera clock offset (camera / +9 etc.)    [camera]:
Subfolder for groups                      [{stem}]:
Also fetch sidecars (gpr,lrv / none / all)[gpr,lrv]:
Existing files (skip/rename/replace)      [skip]:
Chapter renaming (always/multi/never)     [multi]:
Confirm before transferring (y/n)         [y]:

→ saved to ~/Library/Application Support/gpget/config.ini
```

---

## 接続時自動起動

```bash
gpget autostart install       # OS に合わせたトリガーを登録
gpget autostart status        # 有効/無効・mode・転送中かどうか
gpget autostart log           # ログの末尾(現行と .old を通して見る)
gpget autostart log --follow  # 追従
gpget autostart uninstall
gpget autostart install --print   # 登録内容(plist / タスク / unit)を出力するだけ
```

### macOS — ウィンドウは出ない。通知だけが出る

launchd から起動されたバックグラウンドプロセスは、「ローカルネットワーク」プライバシーで
カメラ(172.x)への接続を無音で拒否される。許可プロンプトも出ないし、設定でトグルを ON に
しても効かない。**裸の実行ファイルには、macOS が許可を紐づける「アプリの身元」が
無いため。**

そこで `gpget autostart install` は **`~/Applications/gpget.app`** に小さなアプリ
バンドルを組み立てて ad-hoc 署名し、その**中の** gpget を LaunchAgent から起動する。
身元があるので通信が許可される。配布物は単一バイナリのままで、バンドルはローカルで
組み立てられる。

このバンドルは `LSUIElement` 指定なので、**ウィンドウも Dock アイコンもメニューバー
項目も出ない。**常駐して IOKit の USB 通知(GoPro のベンダー ID `0x2672`)を待つ。
**ポーリングはしない。**ケーブルを挿した瞬間に動きだす。

#### 通知の許可 — ここが一番間違えやすい

初回に許可を 2 つ聞かれるが、見た目がまるで違う。

**ローカルネットワーク**は普通のダイアログ。「許可」を押すだけ。

**通知は右上のバナー**で、タイトルが「gpget」、本文が「テキスト、サウンド、
アイコンバッジにより通知されます」。質問に見えないが、これが許可要求:

- バナーの**「オプション」**メニューを開いて**「許可」**を選ぶ
- **バナー本体をクリックすると設定画面が開くだけで、許可したことにはならない**
- **60 秒で消え、放置すると「拒否」として記録される。**以後 macOS は二度と聞いてこない

**画面を収録している場合は注意。**macOS は画面収録をディスプレイの共有と同じ扱いにし、
**収録に写り込まないようバナーを黙って抑制する。**gpget が壊れているように見えるが、
通知は配信されていて通知センターには入っている。画面に出ないだけ。収録するなら先に
**システム設定 > 通知 > 「ディスプレイをミラーリングまたは共有しているときに通知を許可」**
(一番下、既定 OFF)を ON にする。

見逃したら **システム設定 > 通知 > gpget** を手で ON にする。
`gpget autostart install` は最後にテスト通知を送り、**それが本当に通ったかを表示する**
ので、「許可されている」と「黙って無効」を取り違えずに済む。

`~/Applications` に置く理由: macOS は通知を出す前に Launch Services のデータベースで
バンドルを引く。Launch Services が見ていないディレクトリに置くと、要求はその場で
拒否され、**プロンプト自体が一度も出ない。**

| mode | 動作 |
|---|---|
| **`notify`(既定)** | 未取得件数を数えて通知するだけ(転送しない) |
| **`auto`** | 転送する。進捗は同じ通知を差し替え(約10件ごと、または約15秒ごと)、完了 / 失敗で置き換える。ファイル単位はログへ |

- 通知は gpget.app 自身の名義で出す(`UserNotifications`)。システム設定 > 通知 に
  「gpget」として並ぶ。**Homebrew などの追加インストールは不要**
- **失敗も必ず通知する。**ウィンドウが無いので、黙って終わると成功と区別できない
- アプリ実体:`~/Applications/gpget.app`(再インストールで作り直し)。
  Finder から見える普通のフォルダで、消しても安全(`gpget autostart install` で作り直せる)
- ログは `gpget autostart log` で見る(現行と `.old` を通した末尾。約 1MB で `.old` に1世代)。
  場所は `gpget autostart status` にも出る
- 転送中かどうかは `gpget autostart status`(保存先の `.gpget.lock` を見る)

**既知の macOS 不具合:** gpget を更新するとコード署名が変わり、更新直後の 1 回目の
接続だけが拒否されることがある。gpget は黙って再試行して復帰する(最大 3 回)。
まれに「システム設定 > ローカルネットワーク」に同名項目が複数でき、すべて ON なのに
拒否され続けることがある。この場合の復旧はリカバリーモードが必要で、gpget 側では防げない。

### Windows / Linux — バックグラウンドで完結

プライバシーゲートが無いので、トリガー(タスクスケジューラ / systemd user timer、
どちらも 1 分間隔ポーリング)が `autostart run` を直接叩き、その場で `media/list` を数えて
デスクトップ通知を出す。**未検証**(`--print` で機構を出力できるので手動登録も可)。

| mode | 動作 |
|---|---|
| **`notify`(既定)** | 未取得件数を数えて通知するだけ(転送しない) |
| **`auto`** | 転送する。進捗は同じ通知を差し替え、完了 / 失敗で置き換える。ファイル単位はログへ |

通知:Windows=PowerShell の WinRT トースト、Linux=`notify-send`(通知デーモン必須)、
出せなければ stderr。

### 共通

- 未取得メディアが無い / カメラ未応答なら何もしない。「充電だけ」で挿しても無害
- 同一接続の多重発火は状態ファイル(`os.UserCacheDir()/gpget/autostart*.state`)で抑制。
  ケーブルを抜くと解除され、次に挿すとまた発火する
- 転送の確認: `gpget autostart status`(ロック) と `gpget autostart log`(現行と `.old` を通した末尾。`--follow` で追従)

| OS | ログファイル |
|---|---|
| macOS | `~/Library/Logs/gpget-autostart.log` |
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `${XDG_STATE_HOME:-~/.local/state}/gpget/autostart.log` |

---

## うまくいかないとき

**カメラが見つからない**

```bash
gpget probe        # 見つかった IF / IP、camera/info、media/list の到達可否を表示
```

- **ケーブルがデータ転送に対応しているか。**充電専用ケーブルだと、カメラ側に「USB 接続済み」と出ても PC からは見えない
- カメラの電源が入っているか
- `gpget --ip <addr>` で直接指定できる

**通知が一度も出ない(macOS)**

```bash
"$HOME/Applications/gpget.app/Contents/MacOS/gpget" autostart test-notify
```

PATH の `gpget` ではなく**バンドルの中のコピー**を実行すること。gpget 名義で
通知を出せるのはそれだけ。macOS 自身が申告する設定値が出るので、推測しなくて済む。

```
authorizationStatus  2   (2 = 許可)
alertSetting         2   (2 = 有効)
alertStyle           1   (1 = バナー)
```

これらが正常なのに何も見えないときは、**画面を収録・共有していないか**を確認する。
その間 macOS はバナーを抑制する。設定はシステム設定 > 通知 の一番下。

**Docker や VPN と IP が衝突する**

カメラは `172.16〜172.31` のプライベート帯に現れる。Docker のブリッジや VPN も同じ帯を使うことがある。
gpget は候補ごとに `camera/info` を叩いて GoPro であることを確認してから使うが、
複数残る場合は `--ip` で指定する。

**保存先が拒否される**

上記「保存先に使えない場所」を参照。外付けドライブやローカルディスクを指定する。

**転送が途中で止まった**

```bash
gpget status       # 中断で残った .part を検出して報告する
gpget sync         # 再実行すれば続きから再開する
```

`.part` は勝手に消さない。消したいときは `--clean`。

---

## 関連ドキュメント

| | |
|---|---|
| [docs/design.md](docs/design.md) | なぜ自作か、やること / やらないこと、機能要件、安全モデル |
| [docs/gopro-api.md](docs/gopro-api.md) | GoPro ローカル API の仕様と ILS 実測、TODO |
| [docs/status.md](docs/status.md) | 実装状況・実機検証・未着手 |
| [docs/testing.ja.md](docs/testing.ja.md) | Windows / Linux での動作確認をお願いする手順と、報告の方法 |
| [docs/release.md](docs/release.md) | ビルドとリリースの手順(メンテナ向け) |
| [tools/probe_group.py](tools/probe_group.py) | カメラの API 仕様を実測で確認するスクリプト |

---

## ライセンス

[MIT](LICENSE)

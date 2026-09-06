# gpget

[English](README.md) | **日本語**

GoPro を USB ケーブルで PC につなぎ、撮影素材をディスクに吸い出す。
**microSD を抜くことも、バッテリードアを開けることもない。**

> **コマンドの出力は英語です**。このドキュメントだけが日本語です。

> gpget は独立したツールであり、GoPro 社とは無関係です(提携・出資・承認のいずれもありません)。
> "GoPro" は GoPro, Inc. の商標です。

---

## できること

- **新しい分だけ** — 保存先にまだ無いものを、撮影日ごとのフォルダへ。撮影のたびに走らせるだけ
- **実行をまたいで再開** — Ctrl-C、ケーブル抜け、ノートを閉じた — 再実行すれば、止まったバイト位置から続く。別の日の実行でも
- **転送ファイルが壊れにくい** — 一時名で書き、全バイトが揃ってサイズ確認が済んでから本来の名前になる。途中で止まれば `.part` が残るだけで、半端なファイルが完成品に紛れることはない
- **4GB 超の動画も問題なし** — どんなに大きくても、メモリに載せずそのままディスクへストリームする
- **軽くて速い** — クラウド同期も変換もしない、ファイルを運ぶだけ。「新しい分だけ」と自動起動と合わせれば、撮影後の片付けはあっという間
- **バースト / タイムラプス / インターバルを 1 単位で** — 数百〜数千枚が 1 サブフォルダに、一度の操作で入る
- **チャプター分割 MP4 を名前順に並ぶよう改名** — `GX012495.MP4`, `GX022495.MP4` … を `GX2495_01`, `GX2495_02` に。順番どおり並ぶ(結合はしない)
- **カメラには一切触れない** — 完全リードオンリー。カード上のものを削除・改名・移動するコードは無い。microSD もバッテリードアも閉じたまま

GoPro 公式のツールはスマホアプリと Web のメディアライブラリで、この用途のデスクトップ
アプリはもう無い。gpget はその隙間 — 撮影素材を PC に取り込む部分 — を埋める。

## 動作条件

| | |
|---|---|
| カメラ | **GoPro MISSION 1 PRO ILS** で検証。MISSION 1 系 / HERO11〜13 / MAX2 も、同じメディア一覧を USB で公開していれば動く見込み |
| 接続 | **USB-C ケーブル**(データ転送対応のもの。充電専用ケーブルは不可) |
| macOS | Apple Silicon、macOS 15 以降 |
| Windows | 64bit、Windows 10 以降 |
| Linux | x86_64 または aarch64 |
| 備考 | **レンズや撮影モードは関係ない**。カメラが記録したものをそのまま吸い出す |

**microSD を抜く必要はありません**。メディアモッドを付けたままでも(側面 USB-C がデータを通せば)動きます。

---

## インストール

[Releases](https://github.com/yager/gpget/releases) からバイナリを1つ取るだけです。
ランタイムの導入は要りません。

> **ブラウザでダウンロードしないでください**。macOS では、ブラウザ経由で取得した
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

**日付フォルダはカメラの時計そのまま(記録時の値)**。転送後、ファイルの更新日時も撮影時刻に戻す。

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

ふだんは開かなくて大丈夫 — `gpget init` が要点を設定し、既定のままで問題ない。
ここは何かを変えたくなったときの参照用。

### 場所

設定ファイルは1つ、OS の標準的な場所に置かれる。既定の保存先も OS 固有(下記):

| OS | 設定ファイル | 既定の保存先 |
|---|---|---|
| macOS | `~/Library/Application Support/gpget/config.ini` | `~/Movies/GoPro` |
| Windows | `%AppData%\gpget\config.ini` | `%USERPROFILE%\Videos\GoPro` |
| Linux | `${XDG_CONFIG_HOME:-~/.config}/gpget/config.ini` | `$XDG_VIDEOS_DIR/GoPro`(既定 `~/Videos/GoPro`) |

実パスは `gpget config path` で表示。`--config <path>` で別のファイルを指定でき、
それ以外は見ない。優先順位は `--flag` > `config.ini` > 組み込み既定。

### config.ini

```ini
[general]
dest       = /Users/you/Movies/GoPro    ; OS の動画フォルダ配下(gpget init が実パスを書く)
folder     = {date:%Y-%m-%d}_GoPro     ; 撮影日ごとのサブフォルダ
group_dir  = {stem}                    ; グループ用サブフォルダ(空 = 日付直下)
sidecars   = gpr,lrv                   ; none | gpr,lrv | all
overwrite  = skip                      ; skip | rename | replace
confirm    = true
timezone   = camera                   ; camera = カメラの時刻そのまま / +9 / -05:30 で時計ズレ補正

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

- `folder` / `group_dir` で使える:`{date:%Y-%m-%d}` `{label}`(実行時 `--label`) `{model}` `{cam}` `{stem}`(グループの基底ファイル名。例 `GPAA0015`)
- `chapter_name` で使える:`{prefix}`(`GX` 等) `{clip}`(末尾4桁) `{chapter:02d}` `{date:...}`
- 保存パス = `<dest>/<folder>/<file>`、グループは `<dest>/<folder>/<group_dir>/<frame>`
- **`{date:...}` の基準**:カメラが記録した時刻そのもの。カメラの時計が現地時刻に
  合っていれば正しい。ズレていた/別の時間帯に設定されていた場合だけ、
  `timezone = +9` のようなオフセットで補正する

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

オプション機能で、**`gpget autostart install` を実行するまでは無効**。有効にすると、
カメラを挿すたびに gpget が自分で走るので、取り込みを覚えておく必要がなくなる。

挿したときの動作は config の `[autostart] mode` で決まる:

- **`notify`(既定)** — 未取得の分を数えてデスクトップ通知を出すだけ。転送はしない。
  ファイルが欲しいときは自分で `gpget sync` を実行する
- **`auto`** — 新しい分を確認なしで全部転送し、完了 / 失敗を通知する

どちらの場合も、カメラに新規メディアが無い / 応答しないなら何もしない
(充電目的で挿しても無害)。同一接続での多重発火は、ケーブルを抜くまで抑制される。

カメラの検知方法と、必要になる許可は OS ごとに違う。特に macOS には癖がある。
以下、OS ごとに説明する。

```bash
gpget autostart install       # OS に合わせて設定する
gpget autostart status        # 有効/無効・mode・いま転送中かどうか
gpget autostart log           # 最近のログ
gpget autostart log --follow  # ログを追い続ける
gpget autostart uninstall
gpget autostart install --print   # 登録せず、登録内容だけ出力する
```

### macOS

`gpget autostart install` は **`~/Applications/gpget.app`** を作り、
バックグラウンドで動くよう登録する。これは「gpget をもう一度インストールしたもの」
ではなく、macOS でバックグラウンドサービスを動かすのに必要な形というだけ。
(カメラへの接続と通知の許可は「アプリ」に対して与えられ、素の実行ファイルは
受け取れない。中身は手元の gpget から組み立てる。)

ウィンドウも Dock アイコンもメニューバー項目も出ない。ケーブルを挿してから 1〜2 秒で
反応する。アプリを消しても安全で、`gpget autostart install` で作り直せる。

#### 初回だけ、許可を 2 つ出す

**ローカルネットワーク** — 普通のダイアログが出る。「許可」を押すだけ。

**通知** — 画面右上に**バナー**が出る。タイトルが「gpget」、本文が
「テキスト、サウンド、アイコンバッジにより通知されます」。お知らせに見えるが、
これが許可の要求である。

- バナーにポインタを乗せ、「オプション」メニューを開いて**許可を選ぶ**
- バナー本体をクリックすると設定画面が開くだけで、許可にはならない

`gpget autostart install` は最後にテスト通知を送り、**それが本当に通ったかを表示する**
ので、「許可されている」と「黙って無効」を取り違えずに済む。「許可がない」と出たら
[うまくいかないとき](#うまくいかないとき)を参照。

- 通知は gpget 自身の名義で出る。システム設定 > 通知 に「gpget」として並ぶ。
  **Homebrew などの追加インストールは不要**
- `auto` では進捗通知が同じ場所で更新される(だいたい10件ごと、または15秒ごと)。
  ファイル単位はログへ
- **失敗も必ず通知する**。ウィンドウが無いので、黙って終わると成功と区別できない

### Windows / Linux — バックグラウンドで完結

`gpget autostart install` が、毎分カメラを確認するタスクを登録する
(Windows はタスクスケジューラ、Linux は systemd user timer)。上の `mode` に従って動き、
待っている間は何も表示しない。通知は Windows が PowerShell のトースト、Linux が
`notify-send`(通知デーモンが必要)。どちらも出せなければログへ。

**まだ実機(Windows / Linux)で検証できていません。** 手動の `sync` / `get` は
同じコードなので問題なく、未検証なのは `autostart` だけです。試された方は
[結果を報告](https://github.com/yager/gpget/issues)いただけると助かります
(手順は [docs/testing.ja.md](docs/testing.ja.md))。手動登録したい場合は
`gpget autostart install --print` で登録内容を出せます。

### ログ

`gpget autostart log` で見る(`--follow` で追い続ける)。いま転送中かどうかは
`gpget autostart status`。

| OS | ログファイル |
|---|---|
| macOS | `~/Library/Logs/gpget-autostart.log` |
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `${XDG_STATE_HOME:-~/.local/state}/gpget/autostart.log` |

---

## アップデートとアンインストール

### アップデート

お使いの環境のバイナリをもう一度ダウンロードするだけ(その場で上書きされる)。設定は触られない。

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

aarch64 は `gpget-linux-arm64` を使ってください。

#### Windows(PowerShell)

```powershell
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
gpget version
```

「使用中で上書きできない」と出たら、自動起動タスクが `gpget.exe` を掴んでいます。
先に `gpget autostart uninstall` を実行してからダウンロードし直してください。

**自動起動を使っている場合**は、そのあと `gpget autostart install` をやり直します。
gpget は「接続時に動くもの」の中に自分自身をコピーしているので、そのコピーを
作り直す必要があります。

### アンインストール

```bash
gpget autostart uninstall   # 有効にしていた場合のみ
rm ~/bin/gpget
```

これで自動起動のトリガーと、macOS ではそれが作ったアプリバンドルが消えます。
**設定と、取り込み済みのファイルは残ります。** 設定も消したい場合は、
`gpget config path` が指すフォルダを手で削除してください。

| OS | 設定の場所 |
|---|---|
| macOS | `~/Library/Application Support/gpget/` |
| Windows | `%AppData%\gpget\` |
| Linux | `~/.config/gpget/` |

---

## うまくいかないとき

**カメラが見つからない**

```bash
gpget probe        # 見つかったネットワークと IP、カメラ情報、ファイル一覧が取れるか
```

- **ケーブルがデータ転送に対応しているか**。充電専用ケーブルだと、カメラ側に「USB 接続済み」と出ても PC からは見えない
- カメラの電源が入っているか
- `gpget --ip <addr>` で直接指定できる

**Docker や VPN と IP が衝突する**

カメラは `172.16〜172.31` のプライベート帯に現れる。Docker のブリッジや VPN も同じ帯を使うことがある。
gpget は候補が本当に GoPro かを確認してから使うが、複数残る場合は `--ip` で指定する。

**保存先が拒否される**

上記「保存先に使えない場所」を参照。外付けドライブやローカルディスクを指定する。

**転送が途中で止まった**

```bash
gpget status       # 中断で残った .part を検出して報告する
gpget sync         # 再実行すれば続きから再開する
```

`.part` は勝手に消さない。消したいときは `--clean`。

**GoPro Labs**

Labs でファイル名変更（basename）をしていると、USB 経由で一覧・取り込みができない
可能性があります。Labs 公式も、改名したファイルは USB 転送・Quik・クラウド非対応で、
SD カード直コピー向けだとしています。実際に、標準ファームウェアに戻したら
`gpget list` / `sync` が動くようになった、という報告があります。gpget は Open GoPro
の HTTP（USB ネットワーク）を使うため、Labs で名前を変えている場合はまず標準名に
戻すか、カードを抜いてコピーしてください。

### 自動起動(macOS)

**通知が一度も出ない**

原因は 3 つ。可能性の高い順に。

**1. 許可のバナーを見逃した**。バナーは 1 分ほどで消え、放置すると「拒否」として
記録される。以後 macOS は聞いてこない。手で ON にする。

```
システム設定 > 通知 > gpget
```

**2. 画面を収録・共有している**。macOS は画面収録(QuickTime Player など)を
ディスプレイの共有と同じ扱いにし、**収録に写り込まないようバナーを抑制する。**
gpget が壊れているように見えるが、通知は配信されていて通知センターには入っている。
画面に出ないだけ。**システム設定 > 通知 > 「ディスプレイをミラーリングまたは
共有しているときに通知を許可」**(一番下、既定 OFF)を ON にする。

**3. それ以外**。macOS に直接聞く。

```bash
"$HOME/Applications/gpget.app/Contents/MacOS/gpget" autostart test-notify
```

PATH の `gpget` ではなく**バンドルの中のコピー**を実行すること。gpget 名義で
通知を出せるのはそれだけ。macOS 自身が申告する値が出るので、推測しなくて済む。

```
authorizationStatus  2   (2 = 許可)
alertSetting         2   (2 = 有効)
alertStyle           1   (1 = バナー)
```

**更新したら自動起動が動かなくなった**

gpget を更新するとコード署名が変わり、macOS が別のアプリとして扱うことがある。
たいていはエージェントが自分で再試行して復帰する(`gpget autostart log` に再試行が残る)。

まれに「システム設定 > ローカルネットワーク」に同名項目が複数でき、すべて ON なのに
拒否され続けることがある。**この状態は gpget 側では防げず、直せない。**
復旧にはリカバリーモードが必要になる。

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

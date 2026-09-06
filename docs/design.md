# gpget — 設計と要件

gpget がなぜこの形なのか、何をやって何をやらないか、実装が満たすべき要件。

| | |
|---|---|
| 使い方 | [README](../README.md) |
| カメラ API の仕様と実測 | [gopro-api.md](gopro-api.md) |

---

## なぜ自作 / なぜ Go か

gpget が満たしたい条件は次の3つ。

- **CLI で完結する**。GUI を開かず、スクリプトからも呼べる
- **単一バイナリで配布できる**。ランタイムの導入を利用者に求めない
- **macOS / Windows / Linux で同じように動く**

叩く API は「一覧を取る」「ファイルを GET する」だけで、古くから安定している。
**知人(GoPro 系クリエイター、多くは非開発者)にも配りたい**ので、配布形態が決め手:

- **Go = OS ごとに単一の静的バイナリ**。`gpget.exe` / `gpget` を渡すだけ。`brew` / `scoop` にも載せられる
  - **例外は macOS の自動起動だけ**。配布物は単一バイナリのままで、`autostart install` が
    実行時にバンドルを組み立てる(下記)
- ネットワーク IF 列挙が `net.Interfaces()` で標準ライブラリ(有線 IP 発見に必要)
- GoReleaser + GitHub Actions で mac / Windows / Linux のリリースを自動化

rclone / gh / croc / syncthing が Go なのと同じ理由。

---

---

## スコープ

### やること

- **増分オフロード**(主機能):カメラ内メディアのうち保存先にまだ無いものを、
  撮影日ごとのフォルダへコピー。日付で絞れる。冪等(Ctrl-C しても再実行で継続)
- チャプター分割 MP4 の**リネーム**(連続するファイル名へ。結合はしない)
- グループ(バースト / インターバル)を 1 単位として扱う
- `ls -l` 風のファイル一覧、カード概要(`status`)
- 接続時の自動起動(opt-in)
- 接続診断
- 設定(基本保存先・フォルダ名ルール等)を対話 or `config` サブコマンドで

### やらないこと(設計判断。API・プロトコル上は可能)

| 項目 | できるのに、やらない理由 |
|---|---|
| **カメラ側ファイルの削除** | API は `GET /gopro/media/delete/file?path=<d>/<n>` / `delete/group` / `storage/delete/all` を持つ。gpget は**完全リードオンリー**を不変条件にするため呼ばない。将来 `--delete-after-verified`(検証済み転送後のみ・強いガード付き)を opt-in で足す余地あり |
| チャプター**結合** | `GX01…` + `GX02…` の ffmpeg 連結。分割の不便はリネームで解消できる。結合が要るなら Resolve 等で |
| メタデータ / `udta` 修復 | 直接 GET はバイト完全で GPMF トラックも保持される。結合しない限り不要 |
| GUI(ウィンドウ・画面・アイコン) | CUI で十分。**ただし「.app バンドルを作らない」という意味ではない** — 下記参照 |
| クラウドアップロード | 転送先はローカルフォルダのみ |
| 双方向同期 | 片方向コピーに限定 |

### できないこと(API・プロトコルの制約)

| 項目 | 制約 |
|---|---|
| **BLE でのメディア転送** | BLE は制御・ステータス専用。ファイル転送用の characteristic が無い。BLE の用途は「カメラを起こして WiFi AP を ON にする」だけで、転送自体は HTTP(USB/WiFi)必須 |
| **ハッシュによる内容検証** | API はファイルのハッシュ/ダイジェストを返さない。整合性チェックはバイト数一致まで(TCP/HTTP の整合性には守られる) |
| Range 非対応 FW での再開 | 静的配信が HTTP `Range` を honor する前提。FW/エンドポイントが対応しなければ最初からやり直し |

### 未確定(ハードウェア・アクセサリ由来)

- メディアモッド 2 の側面 USB-C がデータ(ネットワーク)モードを通すか(実機確認待ち。
  充電のみなら転送時はメディアモッドを外す — SD ドアは開けない)
- 有線スループットの上限(実測待ち。WiFi 対応に手を出す価値があるかの判断材料)

---

---

## 要件

### 機能要件

| # | 要件 |
|---|---|
| F-1 | カメラの HTTP API ベース URL を決める。`--ip` / config の `ip` があればそれ。無ければ自動発見:(a) `net.Interfaces()` で `172.(1[6-9]\|2[0-9]\|3[01]).x.x/24` の付いた IF を列挙(Docker の `docker0`/`br-*`、VPN も同帯に入りうる) → 各候補の `.51` に対し **必ず `GET /gopro/camera/info` を叩き、`model_name`/`serial_number` が返る IF だけ採用**(GoPro 確認は必須手順) (b) 採用した候補について serial 末尾3桁 XYZ と `172.2X.1YZ.51` が一致するか検算し、ズレたら警告 (c) 候補が複数残ったら `--ip` を促す。(d) 任意で mDNS `_gopro-web._tcp.local.` も併用可 |
| F-2 | 接続直後に `GET /gopro/camera/control/wired_usb?p=1`(既に有効でも無害) |
| F-3 | `GET /gopro/media/list` を取得・パース |
| F-4 | `GET /videos/DCIM/<dir>/<name>` を**ストリーム**でダウンロード(4GB+ をメモリに載せない) |
| F-5 | **増分判定**:各メディアについて **F-13/F-12 で決まる最終保存先パスを算出し**、そこに既存ファイルがあれば:期待サイズが分かるもの(`s` / `glrv`)は**サイズ一致で skip**、分からないもの(`.GPR` / グループフレーム)は**最終位置に存在すれば skip**(finalize 済み = 完全、という `.part` 不変条件に依拠。必要なら `HEAD` で `Content-Length` 照合)。カメラ側のファイル名では判定しない(保存先には存在しない)。状態ファイルには依存しない(FS が真実) |
| F-5b | **日付・mtime の取得元は `media/list` の `cre` に固定**。`cre` は「カメラの壁時計を UTC のフリして encode した epoch」なので、**`time.Unix(cre,0).UTC()` のフィールドをそのまま採用**(ゾーン変換しない)。`media/info` の `cre` は使わない(動画で二重にズレる / 壊れた値 `cre = 3` あり)。config `timezone` は既定 0(=`camera`)、`+9` 等で時計ズレ補正のみ。転送後 `os.Chtimes` で mtime を撮影時刻に戻す。詳細は `gopro-api.md` の「日付の取得元」 |
| F-5a | **`chapter_name` / `folder` / `group_dir` を変えると、過去に取り込んだ全ファイルが「未取得」に見える**(最終パスが変わるため)。これは仕様。テンプレート変更時は `--dry-run` で影響範囲を確認してから実行する。旧名の掃除は手動 |
| F-6 | 対象の絞り込み:`--date` / `--since` / `--type` /(格下げの)glob。`--dry-run` でプランのみ表示。実行前に「N files / X GB → これらのフォルダ」を出して確認(`-y` で省略) |
| F-7 | 保存先の既存ファイル:`overwrite` = `skip`(既定) / `rename` / `replace`(アトミック置換) |
| F-8 | 転送中は `GET /gopro/camera/keep_alive` を数秒おきに送る(保険。有線・WiFi どちらでも接続を維持するため) |
| F-9 | トランスポート層を分離し、後から `--wifi`(ベース URL を `http://10.5.5.9:8080` に)を足せる構造 |
| F-10 | **保存先プリフライト**:クラウド同期フォルダ(`~/Library/CloudStorage`・iCloud・OneDrive/Dropbox/GoogleDrive パス)/ ネットワークボリューム(SMB/NFS)/ 読み取り専用 を**拒否**。FAT32/16 は最大ファイルが単一ファイル上限に収まる時だけ許可。空き容量を合計必要量と突き合わせて拒否。テストファイルを書いて書き込み可を確認 |
| F-11 | **孤児 `.part` の検出**:`status` で列挙、`sync` で継続、`status --clean` で削除。勝手には消さない。各 `.part` には `<name>.part.meta`(期待サイズ / ソースパス / `cre` / 取得開始時刻)を並置し、**再開時は現在の `media/list` エントリと meta が一致する時だけ継続**。カードをフォーマットして番号が一周し「同名の別ファイル」になっている場合は不一致で弾き、警告して `.part` を残す。`.part` のサイズが期待サイズ以上なら再開せず(壊れている)、検証へ回す |
| F-12 | **グループを 1 単位で転送**:`g`/`b`/`l` でまとまる連番は代表 1 枚ではなく全フレームを取得し、最終保存先 `<dest>/<folder>/<group_dir>/` へ。確認プロンプトはグループで 1 回 |
| F-12a | **フレームの展開**:代表ファイル名の末尾数字列を `\d+$` で分解して prefix と桁数を得て、`b`..`l` を `%0<桁数>d` で生成する。**`m` に含まれる番号は除外する**(除外しないと、欠番のフレームへの要求が 404 になりグループ全体が失敗する)。prefix はグループごとに繰り上がるが**グループ内では固定**なので、この方式で正しく展開できる(実測 80/80) |
| F-12b | **「取得済み」の判定**:期待フレーム集合 = `(b..l) − m`。① 保存先グループディレクトリの**ファイル数**と**合計バイト数**を取り、②「ファイル数 == 期待数」かつ「合計バイト == `s`」なら skip(**`HEAD` を 1 回も投げずに判定できる**。1000 枚でも `stat` だけで済む)。③ 不一致なら、欠けている / サイズの合わないフレームだけを `HEAD` → 転送する。**グループ全体をやり直さない**(フレーム単位で再開可能とする) |
| F-12c | **大きなグループ**:件数で拒否しない(インターバル撮影は 1000 枚超が普通)。ただし閾値を超える場合は件数と合計サイズを提示して確認を取る(`-y` で省略) |
| F-12d | **種別判定**:`t` は使わない(ILS では全モードで `"b"`)。種別は `media/info` の `ct` で判定する(実測 `5` = バースト / `6` = タイムラプス / `10` = インターバル。グループ以外は `0` = 動画 / `3` = ラプス付随動画 / `4` = 単写真)。**この対応は公式に文書化されていない。未知の `ct` は種別不明として扱い、転送処理は `ct` に一切依存させない**(種別は表示ラベルにのみ使う)。ナイトラプスはタイムラプスに統合済みなので 4 つ目の値は出ない見込み |
| F-12e | **`list` の表示**:`[burst ×30]` / `[timelapse ×20]` / `[interval ×30]`、`ct` が未知なら `[group ×N]` にフォールバックする |
| F-13 | **チャプターリネーム**:`PPNNNNNN.MP4` を分解(prefix / 末尾4桁=クリップ / 先頭2桁=チャプター)し、`chapter_name` テンプレートで改名してコピー(バイト無改変)。`regroup` = `always` / `multi`(2章以上のみ) / `never`。**`.LRV` は親 MP4 の最終ファイル名幹に `.LRV` を付けた名前**(例 `GX2495_01.MP4` → `GX2495_01.LRV`)。GoPro の `GL` プレフィックス規則はリネーム後は放棄(MP4 の隣に並んでソートされる方を優先) |
| F-14 | `probe` サブコマンド:発見した IF / IP、`camera/info`、`media/list` 到達可否を表示 |
| F-15 | **接続時自動起動**(opt-in):`autostart install/uninstall/status/print/run`。**macOS** = LaunchAgent で常駐(`RunAtLoad` + `KeepAlive`)し、IOKit の USB 接続通知で反応する(ポーリングなし)。バックグラウンドの launchd プロセスは macOS の Local Network プライバシーでローカル 172.x への接続を無音拒否され許可プロンプトも出せない(裸の実行ファイルにはバンドル identity が無いため)。そこで `autostart install` は `~/Applications/gpget.app` に自前の `.app` バンドルを組み立てて ad-hoc 署名し、その中の実行ファイルを launchd から常駐させる(配布物は単一バイナリのまま)。バンドルは `LSUIElement` 指定でウィンドウも Dock アイコンも出さない。`~/Applications` なのは通知の許可のため — Launch Services が走査しない場所に置くと `usernoted` がバンドルを検証できず、許可プロンプトが一度も出ないまま拒否される(2026-09-06 実測)。`mode` は `notify`=件数通知のみ / `auto`=無音で転送し完了 / 失敗を通知。`auto` の進捗は同じ通知を差し替え、ファイル単位は `gpget autostart log` に残す。**ウィンドウが無いので失敗は必ず通知する**(黙って終わると成功と区別できない)。**Windows** = Scheduled Task 1分ポーリング、**Linux** = systemd user timer 1分ポーリング。Win/Linux はプライバシーゲートが無いので `autostart run` が自分で `media/list` を数え、config `mode` に従い `notify`(通知のみ)/ `auto`(ヘッドレス `sync`)。新規メディアが無い / カメラ未応答なら静かに終了。同一接続の再発火は状態ファイルで抑制。Win/Linux 通知は `internal/notify`(依存ゼロ、`notify-send`/PowerShell toast、無ければ stderr)。**実装済み: macOS は実機検証済み(2026-09-05、接続検出 → バンドル起動 → 106 件 401.8M の無音転送 → 完了通知、進捗差し替えと `autostart log` も確認)・Win/Linux は未検証(`--print` で手動登録可)** |
| F-16 | **やらない。** 手動 `sync`/`get` は TTY 進捗で足りる。自動起動の事後確認は `gpget autostart log`(F-15)。汎用の転送ログは持たない |
| F-17 | **排他ロック**:`sync` / `get` は保存先ごとのロックファイル(`<dest>/.gpget.lock`、PID + 開始時刻)を取ってから走る。既にロックがあれば起動しない(autostart の `auto` と手動実行が同じ `.part` を触る事故を防ぐ)。stale ロック(PID 消滅)は自動で奪う |
| F-18 | **進捗表示**:ファイル単位(転送済 / 合計・%・速度・ETA)と全体(N/M ファイル・合計 GB)を出す。TTY なら 1 行を更新、非 TTY なら行ごと。`--quiet` で抑制、`--json` で機械可読 |

### 安全モデル

- カメラ側は**完全リードオンリー**。削除・変更のコードを持たない
- 転送は保存先に `<name>.part`(+ `<name>.part.meta`)を作って書き込む
- **期待サイズの決め方(優先順)**:
  1. `media/list` の `s`(MP4 / 単体 JPG)
  2. `.LRV` は `glrv`
  3. `.GPR` とグループの各フレームは `s` が無い → **最初のレスポンスの `Content-Length`**
  4. `Content-Length` も無ければ「サイズ不明」。この場合のみバイト数一致検証はできないので、
     ダウンロード完走(エラーなく EOF)を条件に finalize し、**ログに「サイズ未検証」と明記**
- 完了判定は **ダウンロード済みバイト数 == 期待サイズ の完全一致**。1バイトでも足りなければ `.part` のまま残し、完成扱いにしない
- 検証通過後に `.part` → 最終名へアトミックにリネーム(`os.Rename` / 上書き時は同ボリューム内リネーム)。`.part.meta` は削除
- 中断時は `Range: bytes=<既存長>-` で自動再開(回数上限)。**再開の前に `.part.meta` と現在の `media/list` エントリを照合**(F-11)。カメラが Range を返さなければ中断して `.part` を残す
- 中断で残った `.part` は `status` で検出して報告(勝手に消さない)
- エラーは具体的に:空き容量不足 / ドライブが読み取り専用になった / 書き込み中にドライブが外れた / カメラ切断(接続喪失を他エラーと区別)
- 転送完了を確認するまでカメラのメディアは消さない(運用ルール)

### 非機能要件・制約

- **Go(1.21+)。`CGO_ENABLED=0` の単一静的バイナリ**
  - `CGO_ENABLED=0` は必須。Go の内部リンカが darwin/arm64 に ad-hoc 署名を自動で付けるため
    (実測: `flags=0x20002(adhoc,linker-signed)`)。CGO を有効にすると外部リンカに切り替わり
    この保証が消え、Apple Silicon で実行できないバイナリになりうる
- 依存は数個まで:CLI パーサ(軽量)、INI、strftime、必要なら mDNS。中心は stdlib(`net/http` / `net` / `encoding/json` / `os`)
- 対象 OS:macOS(Apple Silicon 主)/ Windows / Linux
- OS 依存は **「IF 発見」「autostart トリガー」「通知」の 3 点のみ**。それぞれ薄いプラットフォーム分岐に閉じ込める
- 配布:GoReleaser + GitHub Actions で 3 OS バイナリ + Homebrew tap + Scoop manifest
- git 管理、OSS 公開想定 → `LICENSE`(MIT。著作権表示は公開時に整える)
- テスト:中断を模擬するローカル HTTP サーバ + ダミーの保存先。実カメラを前提にしない

---

---

## 開発

### 参考

- [Open GoPro HTTP API](https://gopro.github.io/OpenGoPro/http) — 公式仕様。有線接続の IP 式と
  エンドポイント定義はここに準拠している
- ILS で実測した挙動は `docs/gopro-api.md` にまとめている

### 方針

- コアは Open GoPro の media/list → `/videos/DCIM/` DL に、gpget の安全モデル
  (`.part` / バイト一致検証 / Range 再開 / 保存先プリフライト)を被せる
- 並行は最小限(keep_alive の goroutine 1 本、DL は逐次)
- OS 依存 3 点(IF 発見 / autostart / 通知)は `*_darwin.go` `*_windows.go` `*_linux.go` に隔離
- テストは中断を模擬するローカル HTTP サーバに対して行う

---

---

## macOS の .app バンドルについて(重要な注意)

**「GUI をやらない」と「.app バンドルを使わない」は別の話。**ここを混同すると
自動起動の設計を誤る。

### なぜバンドルが要るのか

macOS は「**どのアプリが**ネットワークを使おうとしているか」で通信を許可する。
判定材料になるのが**バンドル identity**(`Contents/Info.plist` の `CFBundleIdentifier` と
コード署名)。

**裸の実行ファイルには identity が無い。**そのため:

- launchd(自動起動)から動かすと、**カメラへの通信が無音で拒否される**
- 拒否された理由も表示されず、許可を求めるプロンプトも出せない
- ターミナルから実行した場合だけ通る(**ターミナルの identity を借りているため**)

### 実測(2026-09-05 / macOS 15.7.7)

| 起動のしかた | カメラへの接続 |
|---|---|
| ターミナルから直接 | OK |
| LaunchAgent → **裸のバイナリ** | **NG(no route to host)** |
| LaunchAgent → **バンドル内のバイナリ** | OK |
| LaunchAgent → `open` でバンドルを起動 | OK |

`osascript` による通知は、いずれの文脈でも成功した。

### ここで言う「バンドル」とは

**ウィンドウもアイコンも出ない。**`Info.plist` に `LSUIElement = true` を書けば、
Dock にもメニューバーにも現れない。**中身は今のバイナリそのまま**で、
macOS が identity を読めるフォルダ構造に置くだけ:

```
gpget.app/Contents/
├── Info.plist        CFBundleIdentifier / LSUIElement / NSLocalNetworkUsageDescription
└── MacOS/gpget       バイナリのコピー
```

**配布物は単一バイナリのまま。**`gpget autostart install` が
`~/Applications/` にこの構造を組み立て、
`codesign -s - --identifier <bundle id>` で ad-hoc 署名する。
置き場所が `~/Applications` である理由は次節(通知の許可)を参照。

### 既知の macOS 不具合(防げない)

バイナリを更新すると**コード署名が変わり、macOS が別のアプリとして扱う**ことがある。
このとき「システム設定 > ローカルネットワーク」に**同じ名前の項目が複数でき、
すべて ON なのに通信が拒否される**。`tccutil` では消せず、
復旧にはリカバリーモードで `/Library/Preferences/com.apple.networkextension.*.plist` を
削除して再起動する必要がある。**AppleCare も macOS 側の不具合と認めている。**

**gpget 側では防げない。**

→ **gpget はウィンドウを出さないため、失敗を必ず通知で可視化する**こと。
これは任意ではなく必須要件(F-15)。無音で止まると、ユーザーには原因も対処も分からない。

### 実装して分かったこと(2026-09-05、実機検証済み)

上の構造だけでは動かない箇所が 4 つあった。いずれも実測で発覚したもの。

**1. `open <app>` は引数を渡さない**

`open` で起動されたバイナリは `os.Args` が空になる。gpget は引数なしを `sync` と
解釈するので、そのままだと**バンドルが対話的な `sync` を始めてしまう**
(stdin が無いので何もせず終わるが、意図した動作ではない)。
`main.go` で「バンドル内から引数なしで起動されたら `autostart run`」と解釈している。
`open --args` に頼らないのは、アプリが既に起動中だと引数が渡らないため。

**2. state ファイルはランチャーと worker で分ける**

ランチャーはネットワークに触れないので**カメラ IP しか知らない**。worker は
`IP|シリアル` を知っている。同じファイルを共有すると、互いに「状態が変わった」と
見えて**10 秒ごとに再発火し続ける**(実測)。`autostart.state`(ランチャー)と
`autostart-worker.state`(worker)に分離した。

**3. 更新直後の 1 回目は必ず拒否される**

バイナリを作り直すとコード署名が変わり、**その直後の 1 回目の接続だけが拒否され、
2 回目以降は通る**(実測で再現)。これを毎回ユーザーに通知すると誤報になるので、
worker は到達できなかったとき**ランチャーの state を消して再発火させ、3 回目まで
黙って再試行する**。それでも駄目なときだけ「ローカルネットワークを確認してください」と
通知する。

**4. worker の制限時間はランチャーと別**

`autostartRun` は元々 90 秒で打ち切っていた。ランチャーの 1 tick としては妥当だが、
worker は**実際の転送**を行う独立プロセスなので、カード 1 枚で 90 秒を軽く超える。
worker のときだけ 6 時間に伸ばした。

**通知が出たかどうかはログに残す。**`notify.Send` は使ったチャンネル名を stderr に
書く(`[notify:System Events] ...`)。autostart 文脈では stderr がログファイルなので、
「通知が出なかった」と「通知を見逃した」を後から区別できる。

### 実機検証(2026-09-05 / MISSION 1 PRO ILS / macOS 15.7.7)

| 検証項目 | 結果 |
|---|---|
| USB 接続 → ランチャーが検出 → バンドル起動 | OK・ウィンドウは一切出ない |
| バンドルからカメラへ接続(ローカルネットワーク) | OK |
| 再発火ループが起きないこと(35 秒 = 3 tick 観測) | OK・1 回だけ |
| 署名変更直後の拒否 → 自動リトライで復帰 | OK・10 秒後に成功 |
| 通知(mode = notify) | OK・System Events 経由 |
| 到達不能時の失敗通知 | OK |
| mode = auto での無音バックグラウンド転送 | OK・106 件 / 401.8M |

### 通知は gpget 自身の名義で出す(2026-09-05、実測で方針変更)

**macOS のバナーは「どのアプリが出したか」で許可を判定し、許可されていなければ
無音で捨てられる。**`osascript` は終了コード 0 を返すので、コード上は成功に見える。
当初 `internal/notify` は `terminal-notifier` → System Events → 素の `osascript` の
順で試していたが、実測すると**どれも表示されなかった**:

| 経路 | 名義 | 実測(macOS 15.7.7) |
|---|---|---|
| System Events 経由の `osascript` | `com.apple.systemevents` | **不可**。バックグラウンド専用アプリで、通知登録一覧(103件)に**存在しない** |
| 素の `osascript` | `com.apple.ScriptEditor2` | **不可**。登録はあるが通知スタイルが「なし」だった |
| `terminal-notifier` | 自身 | 表示される。ただし **Homebrew 依存なので採用しない**(一般ユーザーが入れられない) |
| **`UserNotifications`(gpget.app 自身)** | `com.gpget.gpget` | **表示される。採用** |

バンドルを持つようになったので、普通の Mac アプリと同じ `UNUserNotificationCenter` が
使える。設定に「gpget」として並び、初回に許可を聞かれる。cgo を使うが、macOS ビルドは
どのみち `codesign` が要るのでビルド環境の制約は増えない(Linux / Windows は
`//go:build !darwin || !cgo` 側の no-op に落ちる)。

**踏んだ罠: 許可ダイアログはメインスレッドのランループが回っていないと出せない。**
`dispatch_semaphore_wait` でメインスレッドを止めて待つと、ダイアログを出せないまま
**macOS が自動的に「拒否」を記録し、それが永続化する**。以後 `requestAuthorization` は
常に `Notifications are not allowed for this application` を返し、コードからは解除できない
(ユーザーがシステム設定で ON にするしかない)。`pump()` でランループを回すこと。

**そのため `autostart install` の最後にテスト通知を撃つ。**許可ダイアログを
「ユーザーが画面を見ている install 時」に出すためで、初回のカメラ接続時に
出させると、拒否されても誰も気づけない。

### 通知が一度も許可されない問題(2026-09-06 に原因特定)

新規 macOS アカウントで `install` すると、許可ダイアログが**一度も出ないまま**
`requestAuthorization` が即座に失敗する事象があった。

```
[un] requestAuthorization: granted=NO err=Notifications are not allowed for this application
```

**原因: `usernoted` は、プロセスが自分のバンドル ID を名乗る資格を
Launch Services のデータベースで検証する。**そこに載っていないバンドルは
その場で弾かれ、許可要求そのものが作られない。
`~/Library/Application Support` は Launch Services が走査しないので、
そこに置いた `.app` は永久に載らない。システムログにそのまま出る:

```
E usernoted: LSApplicationRecord failed to find com.gpget.gpget
E usernoted: Failed to validate <path> for use with identifier com.gpget.gpget
E usernoted: Failed to find or validate center with identifier com.gpget.gpget
```

**使い捨てバンドル 13 本での実測(macOS 15.7.7、2026-09-06)。**
バンドル ID を毎回変えれば許可状態は `notDetermined` に戻るので、
同一アカウント上で何度でも初回状態を作れる。

| 置き場所 | LS 登録 | 起動方法 | `LSUIElement` | `NSApplication` | 結果 |
|---|---|---|---|---|---|
| Application Support | なし | 直接 exec | あり | なし | 即拒否・**バナー 0 件** |
| Application Support | なし | `open` | あり | なし | 即拒否・バナー 0 件 |
| Application Support | なし | `open` | **なし** | なし | 即拒否・バナー 0 件 |
| Application Support | なし | `open` | あり | **あり** | 即拒否・バナー 0 件 |
| Application Support | **`lsregister -f`** | 直接 exec | あり | なし | **バナー表示 → 許可** |
| **`~/Applications`** | 自動 | 直接 exec | あり | なし | **バナー表示 → 許可** |

**起動方法・`LSUIElement`・`NSApplication` の有無・ad-hoc 署名はいずれも無関係。**
効いていたのは Launch Services の登録有無だけだった。

対処は **`~/Applications` に置く**こと。Launch Services が自動で走査する場所なので、
登録処理が要らない。私有ツール `lsregister`
(`/System/Library/Frameworks/CoreServices.framework/Frameworks/LaunchServices.framework/Support/lsregister`)
を叩けば任意の場所でも通るが、パスが固定の非公開ツールに依存するので採らない。

判定は目視ではなくシステムログで行う。許可要求は
`com.apple.notificationcenter.askpermissions` という通知レコードとして作られるので、

```
log stream --style compact --level debug --predicate 'process == "usernoted"'
```

を流して `askpermissions` の有無を数えれば、人が画面を見ていなくても白黒つく。

### 許可バナーは 60 秒で失効し、放置は「拒否」になる(2026-09-06 実測)

許可要求が出せるようになっても、それだけでは足りない。macOS の通知許可は
**ダイアログではなくバナー**で来る。タイトルがアプリ名、本文が
「テキスト、サウンド、アイコンバッジにより通知されます」だけで、
**質問に見えない。**しかも:

- 許可は**バナー右下の「オプション」プルダウン**からしか出せない
- **バナー本体をクリックすると設定画面が開くだけ**で、許可にはならない
- **60 秒で自動的に消え、無応答は「拒否」として永続記録される**

実測ログ(バナーを一切触らずに放置):

```
00:25:47.731  Presenting <askpermissions req:"com.gpget.gpgettest13"> as alert
00:26:47.758  Removing displayed <askpermissions ...>        ← ちょうど 60 秒後
その後の authorizationStatus = 1 (denied)
```

つまり**ユーザーが何もしなければ黙って拒否になり、二度と聞かれない。**
アプリ側から解除する手段は無い(システム設定で人が ON にするしかない)。

対処はコードでは不可能なので、**`install` の出力で実物の見た目ごと予告する**。
あわせて `install` は、テスト通知が `UserNotifications` で通ったのか
フォールバック経路に落ちたのかを区別して表示する
(以前は osascript に落ちても「送信しました」と表示しており、
許可が無いユーザーに「大丈夫」と伝えてしまっていた)。

なお `[un] requestAuthorization: timeout` は「**バナーは出たが誰も答えていない**」
ことの徴候で、`granted=NO ... not allowed` の即時失敗(= バナーが出ていない)とは
原因が違う。ログを見るときはこの 2 つを混同しないこと。

### 画面収録中はバナーが出ない(2026-09-06 実測、gpget 側の問題ではない)

新規アカウントでの検証中、**許可済みなのに自動実行の通知が一度も表示されない**
という症状が出た。gpget 側は 5 回とも送信に成功しており(`[notify:UserNotifications]`)、
macOS も受理していたが、表示段階で止めていた。

```
usernoted:          Presenting <com.gpget.gpget ...> as banner
NotificationCenter: <id> (com.gpget.gpget) muted by display state
NotificationCenter: addOrUpdate ... canDisplayWhileCenterIsClosed: false,
                    visibility: [history, alert, muted, lockscreen, ...]
```

**原因は QuickTime Player での画面収録だった。** macOS は画面収録を
「ディスプレイの共有」として扱い、収録に写り込まないよう通知バナーを抑制する。
`donotdisturbd` のログにそのまま出る:

```
Determined whether sharing / mirroring preferences should adjust event behavior; shouldAdjust=1
Resolution modified to accomodate auxiliary state; isScreenMirrored=0 isScreenShared=1 ...
```

`isScreenShared=1` が出ていたのは収録していた 6 分間だけで、収録なしで撮り直すと
`displaying as banner` が出て正常に表示された。設定
(**システム設定 > 通知 > ディスプレイをミラーリングまたは共有しているときに通知を許可**、
既定 OFF)を ON にすれば、収録中でも表示され、収録にも写る(実機確認済み)。

**この症状は gpget からは検出できない。** `UNNotificationSettings` は
`authorizationStatus=2 / alertSetting=2 / alertStyle=1(banner)` と正常値を返す。
公開 API に「いま画面共有で抑制されている」を知る手段が無いので、**ドキュメントで
先に伝えるしかない**(`docs/testing.md` に記載)。テスターが手順を録画しながら試すのは
自然な行動なので、放置すると「通知が出ない」という誤報告が繰り返し上がる。

### 通知まわりの切り分け手順

推測に頼らないための手順。**目視は使わない。**

1. `gpget autostart test-notify` を **バンドル内のコピー**で実行する。
   macOS 自身が申告する `UNNotificationSettings` 全項目が出る(`internal/notify` の
   `Settings()`)。正常なら `authorizationStatus 2 / alertSetting 2 / alertStyle 1`
2. システムログを見る。**管理者アカウントから引くこと**
   (一般ユーザーでは `log show: Could not open local log store: Operation not permitted`)。
   統一ログはシステム全体で共有されるので、別アカウントの事象も管理者側から追える

   ```
   log show --start "<時刻>" --info --predicate 'subsystem == "com.apple.unc"' | grep -i gpget
   ```

   - `Presenting <askpermissions ...> as alert` … 許可要求が出た
   - `LSApplicationRecord failed to find` … Launch Services 未登録(前節)
   - `muted by display state` … 表示が抑制された。次を見る

   ```
   log show --start "<時刻>" --info --predicate 'subsystem == "com.apple.donotdisturb"' | grep -i "Resolution modified"
   ```

   - `isScreenShared=1` … 画面収録 / 共有中
3. `displaying as banner` が出ていれば、実際に画面に出ている

### 自動起動は常駐エージェント + IOKit 通知(2026-09-05)

**ポーリングをやめ、USB 接続イベントで即座に反応する常駐型にした。**

#### 最終形

```
launchd (LaunchAgent, RunAtLoad + KeepAlive, AssociatedBundleIdentifiers)
  └ gpget.app/Contents/MacOS/gpget autostart agent   常駐 1 プロセス
       ・IOKit の USB attach/detach 通知で待つ(CPU 0.0%、RSS 約 11MB)
       ・attach → ネットワーク口を待つ → 転送 → 通知
       ・更新を検出したらバンドルを作り直し、自ら終了(KeepAlive が新コードで再起動)
```

launchd は `open` ではなくバンドル内の実行ファイルを直接起動する(常駐なので `open`
では管理できない)。プロセスをバンドルに結びつけて**ローカルネットワーク許可を初回から
通す**ために `AssociatedBundleIdentifiers` を指定している。

#### マッチ辞書の書き方

`idVendor` は **`IOPropertyMatch` の中に入れる**。トップレベルに置くと何にも一致しない。
実測(macOS 15.7.7、ILS 接続状態):

| マッチ辞書 | 件数 |
|---|---|
| `IOUSBHostDevice` のみ | 18 |
| `IOUSBDevice` のみ | 18 |
| `IOUSBHostDevice` + トップレベル `idVendor` | **0** |
| `IOUSBHostDevice` + `IOPropertyMatch{idVendor}` | **1**(カメラ) |

`IOUSBDevice` は `ioreg -c` には現れないが、マッチ辞書では別名として有効。
発火しない原因はクラス名ではなく `idVendor` の置き場所なので、そこを疑うこと。

#### launchd の LaunchEvents は使わない

正しい辞書で `LaunchEvents` / `com.apple.iokit.matching` を使うと、**約 8 秒ごとに
ジョブが再起動され続けた**。短命プロセスが `xpc_set_event_stream_handler` で
イベントストリームを消費しないため、launchd が配送し続ける。制御されていない
ポーリングと変わらないので、自前で IOKit 通知を持つ常駐型にした。

#### ポーリングと常駐の比較(実測)

| | CPU | メモリ | ウェイクアップ | 反応 |
|---|---|---|---|---|
| ポーリング 5 秒 | 0.068% 常時(1 回 3.4ms) | 瞬間 7.8MB | **5 秒ごと** | 最大 5 秒遅延 |
| 常駐 + IOKit | **0.0%** | 常時 約 11MB | 着脱時のみ | **即時** |

**常駐が重いのは常時メモリを掴む点だけで、CPU とウェイクアップでは常駐が軽い。**

#### 署名変更後の再認証は「回数」ではなく「期限」で待つ

バンドルを作り直すとコード署名が変わり、macOS が再認証するまでローカルネットワークが
拒否される。**復帰までの時間には幅がある(実測で 3〜10 秒)**ため、リトライを回数で
打ち切ると境界で誤報が出る(実際に出した)。`reachRetryFor`(40 秒)まで粘り、
それでも駄目なときだけ通知する。

#### バンドルのバージョンはコピー元バイナリに聞く

自己修復では**古いプロセスが新しいバイナリを包み直す**ので、自分の `version` を
`Info.plist` に書くと版が古いまま残る(実際に残した)。`binaryVersion(exe)` で
コピー元に `version` を尋ねる。


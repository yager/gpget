# 実装ステータス

最終更新: 2026-09-06 / 実機 ILS(FW H26.03.03.00.00)で検証

## Phase 5: autostart(2026-09-05)

| コマンド | 状態 |
|---|---|
| `autostart install / uninstall / status / print` | 実装済み。`status` は保存先ロックを見て転送中かを出す |
| `autostart log [--follow]` | 実装済み。末尾は現行と `.old` を横断。1MB で `.old` に1世代 |
| `autostart run`(接続時に走る本体)| 実装済み。接続の署名を状態ファイルに記録して多重発火を抑制。`mode=auto` はファイル単位をログに残し、進捗通知を同じバナーで差し替える |
| `autostart agent`(macOS の常駐本体)| 実装済み。IOKit の USB 接続通知を待つ。ポーリングなし |
| `autostart test-notify` | 実装済み。macOS では `UNNotificationSettings` の全項目も出力する(通知が出ないときの切り分け用) |

**macOS は他 OS と挙動が違う。** バックグラウンドの LaunchAgent は macOS の
「ローカルネットワーク」プライバシーでローカル 172.x への接続を無音で拒否され、
許可プロンプトも出せない(`net.Interfaces()` は通るが TCP connect が `no route to host`。
nettest で実証)。トグルを ON にしても launchd コンテキストには効かない。
原因は**裸の実行ファイルに「アプリの身元」(バンドル identity)が無い**こと。

→ macOS では、`gpget autostart install` が `~/Applications/gpget.app` に
ad-hoc 署名したバンドルを組み立て、**その中の実行ファイルを launchd が常駐させる**。
中身は gpget のバイナリそのもの。バンドルの身元があるので通信が許可される。
`LSUIElement` 指定なので**ウィンドウも Dock アイコンも出ない**。常駐プロセスは
IOKit の USB 接続通知を待つだけで、**ポーリングはしない**。

**置き場所が `~/Applications` なのは通知のため**。Launch Services が走査しない場所
(以前は `~/Library/Application Support`)に置くと、`usernoted` がバンドルを検証できず、
**許可プロンプトを一度も出さないまま拒否**され、その拒否が永続化する
(2026-09-06 に使い捨てバンドル 13 本で実測)。

詳細と、実装して分かった落とし穴は `docs/design.md` を参照。

| OS | 機構 | 状態 |
|---|---|---|
| macOS | LaunchAgent で `gpget.app` 内の実行ファイルを常駐(IOKit の USB 通知で反応、ポーリングなし)→ 転送 / 通知 | **実機検証済み(2026-09-05)**。接続検出 → バンドル起動 → カメラ到達 → `mode=auto` で 106 件 / 401.8M を無音転送 → 完了通知。ウィンドウは一切出ない。通知は `UserNotifications` で gpget 名義(`authorizationStatus=2`)。進捗差し替えと `gpget autostart log` も同日実機で確認 |
| Windows | Scheduled Task(1分間隔ポーリング、`autostart run` が直接転送/通知)| 実装済み・**実機未検証**。2026-09-06 に「登録できたと出るのに登録されていない」報告があり修正済み(タスク XML を直接渡す方式へ変更、登録後に存在確認)。修正自体は実機で確認できていない |
| Linux | systemd user timer(1分間隔ポーリング、`autostart run` が直接転送/通知)| 実装済み・**未検証**。`--print` で unit を出力、手動登録可 |

- Win/Linux はプライバシーゲートが無いので `autostart run` が自分で `media/list` を叩き、
  未取得件数を数えて `internal/notify`(Linux=`notify-send`、Win=PowerShell トースト、
  無ければ stderr)で知らせる。`mode = auto` は転送 + 進捗通知 + ファイルログ
- ログ確認は `gpget autostart log`。転送中かは `gpget autostart status`
- Win/Linux はイベント駆動(デバイス到着トリガー / udev)を将来対応。現状はポーリング

## 2026-09-06 の修正

- **通知の許可が一度も求められない問題を修正**。`.app` を `~/Applications` へ移した。
  詳細は上の表と `docs/design.md`
- **`install` がテスト通知の結果について嘘をついていたのを修正**。osascript への
  フォールバック(macOS がほぼ握りつぶす経路)でも「送信しました」と表示していた
- **Windows の autostart 登録が無言で失敗するのを修正**(実機未検証)
- **`autostart status` が、実際に使われている `.app` の場所を出すように**。
  更新で移動したのに古い場所を指したままだと、転送は動くのに通知だけ出せない
- `autostart test-notify` が `UNNotificationSettings` を出力するように

**gpget のバグではなかったもの**: 「許可済みなのに自動実行の通知が出ない」という
症状を追ったが、原因は QuickTime での画面収録だった。macOS は画面収録を
ディスプレイの共有として扱い、バナーを抑制する。`docs/design.md` に記録あり

## 2026-09-05 の修正

- **日付フォルダのズレを修正**:`cre` は「カメラ壁時計を UTC のフリで encode」した値。
  ゾーン変換をやめ、`time.Unix(cre,0).UTC()` のフィールドをそのまま使う(`plan.WallClock`)
- **ファイル mtime を撮影時刻に設定**(`os.Chtimes`、finalize 後)
- `timezone` 設定を再定義:`camera`(既定・変換なし)/ `+9` `-05:30`(時計ズレ補正)
- **既定の保存先を OS 固有に**:mac `~/Movies/GoPro` / Win `~\Videos\GoPro` / Linux `~/Videos/GoPro`(XDG)

## できているもの(Phase 1–4)

| コマンド | 状態 | 実機検証 |
|---|---|---|
| `probe` | ✅ | IF 自動発見(`net.Interfaces()` → `172.16-31/24` → 各候補の `.51` に `camera/info` で GoPro 確認)、serial↔IP 検算、`media/list` 到達 |
| `list` | ✅ | `ls -l` 風。サイドカーを親行に畳む(`GX010014.MP4(+LRV)`)、グループは 1 行 + `media/info` の `ct` で種別ラベル(`[timelapse ×20]` 等)、`--new` / `--date` / `--since` / `--video` / `--photo` / `--expand` / `--json` |
| `status` | ✅ | 撮影日ごとの件数・容量・未オフロード量、孤児 `.part` の検出(resumable / STALE)、`--clean` |
| `sync` | ✅ | 増分オフロード。下記「検証済み挙動」 |
| `get <glob>` | ✅ | 名前 / glob でチェリーピック(`sync` と同じパイプライン) |
| `init` | ✅ | 対話ウィザード → `config.ini` |
| `config` | ✅ | `list` / `get` / `set`(バリデーション付き) / `path` / `edit` |

## 検証済み挙動(実機)

- **バイト完全一致**の転送(`GX010014.MP4` = 2,562,074 / `GX010014.LRV` = 138,986、`s` / `glrv` と一致)
- **冪等**:再実行で「nothing to transfer」
- **グループを 1 単位**で転送、`<dest>/<folder>/<stem>/` へ
- **欠番フレーム(`m`)を除外**して展開(`GPAD0099` グループ:カメラで 0100 を削除済み → 0099/0101/0102 のみ取得)
- **中断 → `.part` + `.part.meta` を保持**、壊れた finalize は起きない
- **Range 再開**:中断した `GPAA0023.JPG` を 1,310,720 バイトから継続 → 5,997,814 で完了、`.part` 残らず
- **孤児 `.part` の照合**:meta が現在の media 項目と一致しなければ STALE として弾き、`.part` を残す
- **保存先プリフライト**:`~/Library/CloudStorage/...` を拒否
- **排他ロック**(`<dest>/.gpget.lock`)、**keep_alive goroutine**(3 秒間隔)、**進捗表示**

コードを通読して出た穴のうち、この文書の 1–6 と表の一部は 2026-09-05 後半に直した。
空き容量 0 は満杯として拒否する。`statVolume` 失敗は Windows 未実装のため、従来どおり進む。
汎用転送ログ(旧 F-16)は採らない(手動は TTY、自動は `autostart log`)。Win 30 分制限と Win ロックの stale 奪取は Win 実機のときに直す。

## 未着手 / 未検証

- **チャプターリネーム(F-13)**:実装済みだが、テストカードに複数チャプターのクリップが無い
  (>4GB or 長時間録画が必要)。`regroup` のロジック自体は単体で確認可
- **`overwrite = rename / replace`**:実装済み・未テスト
- **`--wifi`(F-9)**:トランスポート層をまだ分離していない。USB 専用
- **`autostart` の Win/Linux**:機構は実装済みだが実機未検証
- **単体テスト**:中断模擬のローカル HTTP サーバによるテストは `internal/xfer/download_test.go` にある。`plan` の have 判定と orphan のグループフレームもカバー
- **配布**(GoReleaser / brew / scoop):未着手
- Windows の `statVolume`(空き容量・FS 種別チェック)は未実装
  (クラウドパス denylist と実 test-write は動く)

## コード

- Go / 外部依存ゼロ / `go build ./...` は darwin・windows・linux で通る
- 約 3,700 行、`internal/{gopro,config,plan,xfer}` + `cmd_*.go`

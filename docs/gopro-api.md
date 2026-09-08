# GoPro ローカル HTTP API — 仕様と ILS 実測

GoPro のカメラを USB / WiFi 経由のローカル HTTP API で操作するときの仕様メモ。
**gpget の実装のために調べたものだが、gpget に依存しない内容として書いている。**

主な検証機:**GoPro MISSION 1 PRO ILS**(FW `H26.03.03.00.00`)。
他機種で異なる可能性がある箇所は都度明記している。

公式仕様: <https://gopro.github.io/OpenGoPro/http>

この文書は 3 部構成:

1. **[仕様](#1-仕様)** — 安定して依拠してよい内容
2. **[実測ログ](#2-実測ログ)** — 日付つきの検証記録。仕様の裏付け
3. **[TODO](#3-todo)** — まだ確認できていないこと

---

## 1. 仕様

### エンドポイント

| 用途 | リクエスト |
|---|---|
| 有線制御 ON | `GET http://<ip>:8080/gopro/camera/control/wired_usb?p=1`(接続直後に送る) |
| メディア一覧 | `GET http://<ip>:8080/gopro/media/list` → 下記スキーマ |
| ファイル取得 | `GET http://<ip>:8080/videos/DCIM/<d>/<n>`(静的配信・バイト完全・`Range` 対応。数十 KB チャンクでストリーム) |
| GPMF サイドカー | `GET http://<ip>:8080/gopro/media/telemetry?path=<d>/<n>`(直接 DL でトラックは保持されるので通常不要) |
| ファイル個別情報 | `GET http://<ip>:8080/gopro/media/info?path=<d>/<n>` |
| キープアライブ | `GET http://<ip>:8080/gopro/camera/keep_alive`(gpget では**使わない** — 有線 USB では不要。下記実測ログ) |
| 機種情報 | `GET http://<ip>:8080/gopro/camera/info`(シリアル・FW・機種名) |
| カメラ時計 | `GET http://<ip>:8080/gopro/camera/get_date_time` → `{"date":"2026_09_04","time":"20_50_19","tzone":540,"dst":0}`(`tzone` は分。参考表示用。`cre` は既に壁時計なので日付判定には不要) |
| モード構成(参考) | `GET http://<ip>:8080/gp/gpControl`(`modes` / `ui_mode_groups`。**UI 非表示のモードも見える**) |
| プリセット一覧(参考) | `GET http://<ip>:8080/gopro/camera/presets/get`(**カスタムモードを含む**。`userDefined` / `isModified`) |
| 現在の状態(参考) | `GET http://<ip>:8080/gopro/camera/state`(`status["97"]` = 現在のプリセット id) |
| 削除(gpget では**使わない**) | `GET .../gopro/media/delete/file?path=<d>/<n>` ほか |

- WiFi(AP モード):カメラ `10.5.5.9:8080`。PC を GoPro の SSID に手動接続する必要あり(ネット断)
- 仕様:<https://gopro.github.io/OpenGoPro/http>

#### media/list のスキーマ

```json
{ "id": "<session id>",
  "media": [ { "d": "100GOPRO",
    "fs": [
      { "n": "GP010009.JPG", "s": "2190773", "cre": "<epoch>", "mod": "<epoch>", "raw": "1" },
      { "n": "GX010014.MP4", "s": "2562074", "cre": "...", "mod": "...", "glrv": "138986", "ls": "-1" },
      { "n": "GPAA0015.JPG", "s": "119910904", "g": "1001", "b": "15", "l": "34", "t": "b", "m": [] }
    ] } ] }
```

| フィールド | 意味 |
|---|---|
| `n` / `s` | ファイル名 / サイズ(バイト・**文字列**) |
| `cre` / `mod` | 作成 / 更新 epoch(文字列) |
| `raw: "1"` | JPG に `.GPR` サイドカーあり。**一覧には別行で出ない** |
| `glrv` | MP4 の `.LRV` プロキシのサイズ。一覧には別行で出ない |
| `ls` | MP4 の属性(`-1` = 通常) |
| `g` | グループ ID(`1001`, `1002`, … グループごとに増える) |
| `b` / `l` | グループ内の最初 / 最後のフレーム番号(= ファイル名末尾 4 桁) |
| `m` | 欠番 / 削除されたフレーム番号の**文字列配列**(`["100"]`。**ゼロ詰めしない**)。無ければ `[]`。**展開時に必ず除外すること**(除外しないと削除済みフレームで `404`)。`s` も欠番を除いた合計に再計算される |
| `t` | **ILS では常に `"b"`。種別判定に使えない**(公式 SDK は `b`/`c`/`n`/`t` としているが ILS は `b` のみ) |
| グループ項目の `s` | **全フレームの合計バイト数**(実測 3 グループで一致)。公式 SDK の「ファイル数」というコメントは誤り。**フレーム単位の期待サイズは `HEAD` の `Content-Length`** |
| `ct`(`media/info` のみ) | 撮影モードの手がかり。実測 `0` = 通常動画 / `3` = ラプス付随動画 / `4` = 単写真(**スタートレイル合成も同値**) / `5` = バースト / `6` = タイムラプス / `10` = インターバル。**未文書。未知の値は種別不明として扱う** |

#### GoPro ファイル名の構造

`PPNNNNNN.MP4` = 8 文字:

- `PP` = プレフィックス(`GX` = HEVC / `GH` = AVC / `GL` = LRV / `GS` = 360)
- 先頭 2 桁 = **チャプター番号**(01, 02, …)
- 末尾 4 桁 = **クリップ番号**(同一録画のチャプター間で共通)

名前順に並べると 1 録画のチャプターが他クリップに割り込まれてバラける
(`GX012495` … `GX012496` … `GX022495`)。F-13 のリネームで解消する。

#### サイドカー / グループの取得ルール

| 種類 | 条件 | 変換 | 実測 |
|---|---|---|---|
| RAW | JPG entry に `raw:"1"` | `GP010011.JPG` → `GP010011.GPR` | `200` / 10,054,480 B |
| LRV | MP4 entry に `glrv` | `GX010014.MP4` → `GL010014.LRV`(`GX`→`GL`) | `200` / 138,986 B(= `glrv`) |
| グループ全フレーム | entry に `g`/`b`/`l` | 代表名の末尾数字列を正規表現 `\d+$` で分解 → prefix + 桁数。`b`..`l` を `%0<桁数>d` で展開し、`m` を除外して個別 GET | 4 グループ 80/80 で `HEAD` `200` |

- MP4 DL のバイト数は `s` と完全一致(`GX010014.MP4` → 2,562,074 B)
- `Range: bytes=0-1023` → `206` / 1024 B

---

#### カメラのモード / プリセット構造

**gpget の転送処理はこれらに一切依存しない。**`probe` の診断表示と、機種差を理解するための参考。

##### 3 つの層がある

| 層 | エンドポイント | 内容 | 「モードの管理」の影響 |
|---|---|---|---|
| モード | `GET /gp/gpControl` の `modes` | 撮影エンジン 18 種 | **受けない**(非表示でも見える) |
| グループ | `GET /gp/gpControl` の `ui_mode_groups` | モードの束ね方 5 群 | **受けない** |
| **プリセット** | `GET /gopro/camera/presets/get` | **ユーザーが実際に選ぶ実体** | **受ける(有効化したものだけ返る)** |

**可視性を知りたいときに見るのは `presets/get`。**`ui_mode_groups` は「カメラが持つ構造」であって
「今 UI に見えているもの」ではない。ILS は **「モードの管理」で個々のプリセットを表示 / 非表示**
にでき、ナイトエフェクト / Vlog / ループ / オープンゲート / 長時間 が既定で非表示。

**検証済み**:非表示 4 つを有効化すると `presets/get` は 10 → 14 に増え、
`modes`(18)と `ui_mode_groups`(5)は**変化しなかった**。

##### `modes`(ファームが持つ全モード。非表示でも見える)

| id | 名前 | id | 名前 |
|---|---|---|---|
| 4 | Playback | 24 | Motion |
| 5 | Setup | 27 | Slo-Mo |
| 12 | Standard | 28 | Idle |
| 13 | Stationary | 29 | Star Trails |
| 15 | Looping | 30 | Light Painting |
| 16 | Single | 31 | Vehicle Lights |
| 17 | Interval | 32 | Burst Slo-Mo |
| 19 | Burst | 36 | Low Light |
| 22 | Broadcast Record | 23 | Broadcast |

**「Night Lapse」「Time Lapse」という名前のモードは存在しない**。タイムラプス系は
Motion(24)/ Stationary(13)の 2 つ。公式スペックの
"Night Lapse — Consolidated With Time Lapse" と一致する。

##### プリセット全 14(すべて有効化した状態)

| `titleId` | `mode` | id | 備考 |
|---|---|---|---|
| `PRESET_TITLE_VIDEO` | `FLAT_MODE_VIDEO` | 0 | 変更済み |
| `PRESET_TITLE_PHOTO` | `FLAT_MODE_PHOTO` | 65536 | 変更済み |
| `PRESET_TITLE_LAPSE` | `FLAT_MODE_TIME_LAPSE_VIDEO` | 131072 | 変更済み |
| `PRESET_TITLE_NIGHT_EFFECTS` | `FLAT_MODE_VIDEO_STAR_TRAIL` | 196608 | 既定で非表示 |
| `PRESET_TITLE_SLOMO` | `FLAT_MODE_SLOMO` | 262144 | |
| `PRESET_TITLE_SPORT_POV` | `FLAT_MODE_VIDEO` | 327680 | |
| `PRESET_TITLE_VLOG` | `FLAT_MODE_VIDEO` | 458752 | 既定で非表示 |
| `PRESET_TITLE_ENDURANCE` | `FLAT_MODE_VIDEO` | 589824 | 既定で非表示 |
| `PRESET_TITLE_LOW_LIGHT` | `FLAT_MODE_VIDEO_NIGHT` | 720896 | |
| `PRESET_TITLE_LOOPING` | `FLAT_MODE_LOOPING` | 786432 | 既定で非表示 |
| `PRESET_TITLE_OPEN_GATE` | `FLAT_MODE_VIDEO` | 851968 | 既定で非表示 |
| `PRESET_TITLE_MOUNTED` | `FLAT_MODE_VIDEO` | 1638400 | **userDefined** |
| `PRESET_TITLE_USER_DEFINED_CUSTOM_NAME` | `FLAT_MODE_VIDEO` | 1703936 | **userDefined** |
| `PRESET_TITLE_CUSTOM_CINEMATIC` | `FLAT_MODE_VIDEO` | 1769472 | **userDefined** |

ILS ではすべて `PRESET_GROUP_ID_VIDEO` の 1 グループに入る(写真もラプスも)。
HERO 系の 3 グループ構成とは違うので、他機種対応時の注意点。

**id は 65536(0x10000)の倍数**に並ぶ(0x00, 0x01, 0x02, 0x03, 0x04, 0x05, 0x07, 0x09,
0x0B, 0x0C, 0x0D … カスタムは 0x19 以降)。**欠番(0x06 / 0x08 / 0x0A)がある**ので、
他機種や将来 FW で別のプリセットが入る余地がある。**id を決め打ちしないこと。**
カスタムかどうかの判定には `userDefined` フラグを使う(id の範囲で判定しない)。

##### `camera/state` の関連 status

| キー | 意味 | 検証 |
|---|---|---|
| `status["97"]` | **現在のプリセット id** | 2 回一致(`196608` = NIGHT_EFFECTS / `1703936` = カスタム) |
| `status["89"]` | **現在のモード id** | 2 回一致(`29` = Star Trails / `12` = Standard) |
| `status["96"]` | プリセットグループ id と思われる | 2 回とも `1000` で不変。ILS はグループが 1 つなので判別できない |

`FLAT_MODE_*` とモード id の対応は、上記 2 例(`FLAT_MODE_VIDEO` → 12 /
`FLAT_MODE_VIDEO_STAR_TRAIL` → 29)が実測。**残りは名前からの推定**
(`FLAT_MODE_LOOPING` → 15 / `FLAT_MODE_VIDEO_NIGHT` → 36 / `FLAT_MODE_SLOMO` → 27 /
`FLAT_MODE_PHOTO` → group 1001 / `FLAT_MODE_TIME_LAPSE_VIDEO` → group 1002)。

##### gpget での扱い

- **転送・検証・命名のロジックはモード / プリセットに一切依存させない。**
  カスタムプリセットはユーザーごとに違い、表示 / 非表示も機種も変わる
- `probe` で「モード数 / UI グループ / プリセット一覧(カスタム・非表示状態を含む)」を
  表示すると、他機種対応やユーザーからの不具合報告の切り分けに役立つ
- **プリセットが増減しても media 側の扱いは変わらない。**
  出力の種別は `media/list` / `media/info` 側(`g` / `ct` 等)だけで判断する

---

---

## 2. 実測ログ

日付つきの検証記録。上の「仕様」の裏付け。

#### 実機確認済み(2026-09-04 / ILS)

- ILS を USB-C で Mac(mac mini)に接続 → `en10` に有線ネットワーク IF が上がる
- **ILS は Open GoPro HTTP API v2.0 を喋る**。`GET /gopro/version` → `{"version":"2.0"}`
- `GET /gopro/camera/info` → `model_number: 74` / `MISSION 1 PRO ILS` / FW `H26.03.03.00.00` /
  `serial` と `ap_ssid`(`GoPro <シリアル末尾8桁>`)も返る
- `GET /gp/gpControl` も応答(モード一覧の JSON)
- **IP 導出式が的中**:serial 末尾 3 桁を `XYZ` として `172.2X.1YZ.51`。
  実測機では `172.23.188.51` で、ホスト側は同じ /24 の別アドレス
- `wired_usb?p=1` → `200`、`keep_alive` → `200`(どちらも ILS で有効)。enable 無しでも短時間は接続維持
- `media/list` 取得 OK。`GET /videos/DCIM/100GOPRO/<n>` DL のバイト数は `s` と完全一致
- `Range: bytes=0-1023` → `206` / 1024 B(再開に使える)
- `.GPR` / `.LRV` はサイドカー名変換で取得可(下記)
- 検証は **bare の USB-C ポート**で実施(メディアモッド 2 経由は未検証)
- 注意:テスト時は `.56`(自機)や `localhost` を叩かないこと。ローカル Docker 等が :8080 を専有していてノイズになる

##### グループ(バースト / インターバル / タイムラプス)の実測 — 2026-09-05

`tools/probe_group.py` で 4 グループ(20 / 30 / 30 / 4 枚)を検証:

- **グループ項目の `s` は「全フレームの合計バイト数」。** 3 グループ 80 枚すべてで
  各フレームの `Content-Length` の総和と**完全一致**(119,910,904 / 97,330,750 / 76,204,116)。
  **公式 SDK(`open_gopro/models/media_list.py`)は `s` を "Number of files in the group" と
  コメントしているが、これは誤り**(少なくとも ILS FW `H26.03.03.00.00` では)
- **`t` は撮影モードを問わず常に `"b"`。種別判定に使えない**(タイムラプス / バースト /
  インターバルすべて `"b"`)
- **種別の手がかりは `media/info` の `ct`**(値の一覧は下表)
- **ナイトラプスは存在しない。** 公式スペックが
  **"Night Lapse — Consolidated With Time Lapse"**(同様に Night Photo / Night Shutter も統合)と
  明記。→ 4 つ目の `ct` は原理的に出ない
- **プレフィックスはグループごとに繰り上がる**(`GPAA` → `GPAB` → `GPAC` → `GPAD`)。
  数字部分は全体で連番。**1 グループ内では prefix 固定・末尾 4 桁が `b`..`l`**
- **静的配信エンドポイントは `HEAD` に応答し `Content-Length` を返す**(80/80 成功)。
  フレーム単位の期待サイズ取得に使える
- **`m`(欠番)の仕様を確認済み**(グループ内の 1 枚をカメラで削除して検証):
  - `m` は**文字列の配列**。**ゼロ詰めしない数値**が入る(`"m": ["100"]`。`b` / `l` と同じ表記)
  - **`s` は欠番を除いた合計に再計算される**(22,924,522 → 17,191,943。差 5,732,579 は
    削除した `GPAD0100.JPG` のサイズと完全一致)
  - `(b..l) − m` = 3 枚を展開 → `HEAD` 3/3 成功 → 合計 17,191,943 で **`s` と完全一致**
  - **削除済みフレームへの GET / HEAD は `404`**。前後のフレームは `200`
  - → **`m` を無視して `b`..`l` を全部叩くと 404 で失敗する**。欠番のあるグループを
    扱うクライアントは、展開時に `m` の番号を必ず除外すること

`media/info` の `ct` の実測値(ILS FW `H26.03.03.00.00`):

| `ct` | 意味 | 実例 |
|---|---|---|
| `0` | 通常の動画 | GX010036 / GX010037(27MB / 50MB の実録画) |
| `3` | ラプス系に付随する動画出力 | GX010014 / GX010098 / GX010103(グループや合成写真と同時刻・小サイズ) |
| `4` | **単写真**。**スタートレイルの合成結果も同じ `4`** | GP010009〜GP010035 / GP010104 |
| `5` | バースト(グループ) | GPAB0038 |
| `6` | タイムラプス(グループ) | GPAA0015 / GPAD0099 |
| `10` | インターバル(グループ) | GPAC0068 |

バーストとインターバルは**同じ 4096×3072 なのに `ct` が違う**ので、`ct` は解像度由来ではなくモード由来。
**ただし `ct=4` は「普通のシングル写真」と「スタートレイルの合成結果」を区別しない。**
`ct` は表示ラベルの補助に留め、**転送処理を `ct` に紐づけないこと**(F-12d)

##### 日付の取得元 — `media/list` の `cre` を「壁時計」として使う(重要 / 2026-09-05 訂正)

**`media/list` の `cre` は「カメラの壁時計を UTC のフリして encode した epoch」。**
ゾーン変換してはいけない。`time.Unix(cre,0).UTC()` の年月日時分秒がそのまま
カメラの表示時刻(= カメラの時計が現地時刻なら現地の壁時計)。

実測(カメラ時計 JST、実際に 9/4 16:40 に撮った GX010098):

| 読み方 | 結果 | 正誤 |
|---|---|---|
| `time.Unix(cre,0).UTC()` | 09/04 16:40 | ✅ カメラ表示と一致 |
| `time.Unix(cre,0).In(JST)`(ゾーン変換) | 09/05 01:40 | ❌ +9h ズレて日付が翌日に飛ぶ |

`mod`(更新時刻)も `cre` と同じ encode。

`media/info` の `cre` は別物:**動画では `cre − 32400`(壁時計を UTC 扱い、さらに
JST 変換で二重にズレる)を返し、1 件は壊れた値(`cre = 3`)を返した。**
写真は `media/list` と概ね一致。→ **日付にも mtime にも `media/list` の `cre` だけを使い、
`.UTC()` のフィールドをそのまま採用する。`media/info` の `cre` は使わない。**

gpget: `plan.WallClock(cre, offset)` が `time.Unix(cre,0).UTC()` を local ゾーンで
再構築して返す。`offset`(config `timezone`)は既定 0。カメラの時計が現地とズレて
いた場合だけ `+9` 等で補正する。転送後 `os.Chtimes` で mtime を撮影時刻に戻す。

##### USB リンク速度と転送スループット(2026-09-07〜08 / ILS + Mac mini M4 Pro)

**リンク速度(実測、`system_profiler SPUSBDataType` の `MISSION 1 PRO ILS` /
`0x2672:0x005a`):**

| ケーブル | 表示 |
|---|---|
| GoPro 純正(付属) | `Speed: Up to 480 Mb/s` — USB 2.0 配線のみ(SuperSpeed ペア無し。ケーブルチェッカーでも確認) |
| USB 3 ケーブル | `Speed: Up to 10 Gb/s` |

**ILS 本体は USB 3.2 Gen 2(10 Gbps)対応。** このドキュメントが以前前提にしていた
「歴代 GoPro は USB 2.0 だから ILS も 2.0」は**誤り**だった。ただし下記のとおり
転送速度はリンク速度で決まらない。

**転送スループット(実測):**

- **大ファイル 1 本を 1 接続で連続取得 → 40.6 MiB/s で完全に安定。** 1 GiB を
  128 MiB 刻みで計測し全区間 40.2〜40.6。**ケーブル種別に依存しない**(USB2/USB3 で
  同じ)。熱・SD・バッファによる劣化は無し
- **インターバル群(330 フレーム / 3 GB)→ 実効 ≈ 22 MiB/s。** 1 ファイル 1 接続の
  確立コストで大ファイルの約半分。前段の per-frame `HEAD` も加わる(~85 ms/frame、
  330 枚で ~28 秒)
- → **~40 MiB/s(≈ 340 Mbps)が Open GoPro HTTP 転送の素の上限。** USB リンク
  (10 Gbps)は全く律速ではない。律速は microSD 読み出し → カメラ SoC の HTTP 配信
  → USB 上の擬似イーサネット(NCM)トンネル。**この方式である限りケーブルも PC も
  変えて速くならない。** カードリーダー(V30 UHS-I で ~90 MB/s、上位で 150+)の方が
  生の MB/s では 2〜4 倍速い

**HTTP keep-alive: 非対応。** レスポンスごとに接続を閉じる(同一接続で 2 回目の
リクエストを送ると `RemoteDisconnected`)。→ gpget の keep_alive pinger を撤去した
(2026-09-08)。有線 USB では給電中カメラが起きたまま(実測)で、転送自体が
トラフィックなので 3 秒ごとの keep_alive は仕事が無く、「1 接続ずつしか捌かない」
カメラに第 2 接続を足していただけ。GPTransfer(先行実装)も keep_alive も
`wired_usb?p=1` も送っていない。

**転送途中の停止(部分的に観察、機構は推測):**

- 9GB/4K を録画した**直後**、小ファイルを連続取得すると、一部の転送が body の途中で
  **7 / 15 / 28 / 60 / 120 / 420 秒**フリーズし、その後 full speed で再開する現象を
  観察(curl と使い捨てスクリプトで計測。`tools` 未収録)。停止時間の値は TCP 指数
  バックオフ(1+2+4+8+…)と一致 → **NCM トンネルでのパケットロス + TCP 再送
  タイムアウト**が機構と推測
- **TTFB(最初のバイト)は停止時も 0.03 秒で一定。** 詰まるのは接続確立ではなく
  body の途中
- **休ませたカメラでは再現しない。** インターバル群の before/after A/B(30 フレーム
  × 8 回、330 フレーム × 2 回)で停止ゼロ、全ファイル 0.5 秒未満。連続録画と違い
  インターバル撮影は SoC をほぼ加熱しない
- → **停止は実運用ではレアで、ヘビーな録画の直後・発熱時に限られる(推測)。**
  `overheat` フラグとの相関は未確認(A/B 中に `/gopro/camera/state` から
  overheat を読めなかった。エンドポイント自体は `status["97"]` 等を返す)
- gpget 側の対応:`stream()` の body 読み取りに 15 秒のアイドルデッドライン。
  無応答なら接続を切って Range 再開(既存の再試行ループを流用)。詳細は
  `docs/status.md` と `internal/xfer/idle.go`

### 手動接続手順

```bash
ifconfig | grep 'inet 172\.'
```

自機に `172.A.B.C/24` が付く。**カメラはそのサブネットの `.51`**(自機 IP ではない)。

```bash
CAM=172.23.188.51                       # ← 実際の値に
curl -s "http://$CAM:8080/gopro/camera/info"
curl -s "http://$CAM:8080/gopro/media/list" | python3 -m json.tool | head
```

`localhost` / 自機 IP を叩かない(ローカル Docker 等が :8080 を占有していることがある)。

---

#### 検証スクリプト

`tools/probe_group.py` — カメラに接続した状態で実行すると、`media/list` のグループ項目を
展開して各フレームに `HEAD` を投げ、`s` が「合計バイト」「ファイル数」「代表 1 枚」の
どれと一致するかを判定する。あわせて `media/info` の `ct` と `cre` を突き合わせ、
`media/list` との食い違いを検出する。**FW 更新時や他機種で仕様が変わっていないかを
再確認するために使う**(読み取りのみ。ダウンロードはしない)。

```bash
python3 tools/probe_group.py [カメラIP]   # 既定 172.23.188.51
```

---

## 3. TODO

リリースまでに潰す。GitHub Issue には出していない。

### まだ確認できていないこと

- [ ] 無通信のまま何分で HTTP サーバが無応答になるか(keep_alive は撤去済み。
      `sync` の確認プロンプトで長時間放置 → 初回転送が失敗する可能性。未再現)
- [ ] **フレーム番号が 9999 を超えたときの挙動** — 現在 102 番。当面先
- [ ] メディアモッド 2 の側面 USB-C でも同じか

**解決済み**(上の実測へ移動):バーストの全フレーム名(prefix はグループごとに繰り上がる /
末尾 4 桁ゼロ詰め)、グループの `s` の意味、`t` の値域、`m`(欠番)の形式と挙動、
スタートレイル等の出力形式(グループにならない)、**実効スループット(大ファイル
~40 MiB/s / インターバル ~22 MiB/s、ケーブル種別に依存しない)**、**USB リンク速度
(ILS は USB 3.2 Gen 2)**

---

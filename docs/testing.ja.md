# テストのお願い（Windows / Linux）

[English](testing.md) | **日本語**

gpget は、GoPro を USB でつないで写真・動画を PC に取り込むコマンドです。
**Windows と Linux では一度も動かせていません。**開発は Mac で行っていて、
手元に確認できる環境がありません。そこをお願いしたい、という文書です。

うまく動かなくても構いません。**動かなかったという報告そのものが目的**です。

- 所要時間: 15〜30分程度
- 必要なもの: GoPro(USB でつなげるもの)、USB ケーブル、空き容量
- **カメラの中身は一切変更しません。**読み取り専用で、削除もしません

---

## 1. インストール

### Windows (10 / 11、64bit)

PowerShell を開いて、次を1行ずつ実行します。

```powershell
mkdir "$env:USERPROFILE\bin" -Force
curl.exe -L -o "$env:USERPROFILE\bin\gpget.exe" https://github.com/yager/gpget/releases/latest/download/gpget-windows-amd64.exe
$env:Path += ";$env:USERPROFILE\bin"
gpget version
```

最後に `gpget v0.1.0` のように出れば成功です。

> **`$env:Path` の変更はそのウィンドウだけ有効です。**新しい PowerShell を開くと
> 消えます。恒久的にしたい場合は「システム環境変数」から `%USERPROFILE%\bin` を
> Path に追加してください。追加しなくてもテストはできます。

> Windows が「発行元を確認できません」と警告する可能性があります。
> **これが出るかどうかも知りたい情報です。**出たらその画面を教えてください。

### Linux (x86_64)

```bash
mkdir -p ~/bin
curl -L -o ~/bin/gpget https://github.com/yager/gpget/releases/latest/download/gpget-linux-amd64
chmod +x ~/bin/gpget
export PATH="$HOME/bin:$PATH"
gpget version
```

### Linux (Raspberry Pi など aarch64)

上の URL の `gpget-linux-amd64` を **`gpget-linux-arm64`** に変えてください。
どちらか分からない場合は `uname -m` で確認できます
(`x86_64` → amd64、`aarch64` → arm64)。

---

## 2. 最初の設定

```
gpget init
```

対話形式で、**保存先フォルダ**などを聞かれます。空き容量のある場所を指定してください。
それ以外は既定のままで構いません。

---

## 3. 試していただきたいこと

### (1) カメラを認識するか

GoPro の電源を入れ、USB で PC につなぎます。**カメラ側に「USB 接続済み」などが
出るまで数秒待ってから**、次を実行します。

```
gpget probe
```

うまくいけば、カメラの機種名・シリアル・ファームウェア等が並びます。
**ここで失敗する場合、以降は進めません。**その時点で報告してください。

### (2) カードの中身が見えるか

```
gpget status
gpget list
```

### (3) 取り込めるか

```
gpget sync
```

確認を求められたら `y` を入力します。進捗が1ファイルずつ表示され、
最後に `transferred N, skipped 0, failed 0` のように出れば成功です。

**取り込んだファイルが実際に開けるか**も確認してください
(写真が表示できる、動画が再生できる)。ファイルサイズだけ合っていて
中身が壊れている、という事故を見つけたいためです。

### (4) 自動起動（ここが一番不安な部分です）

```
gpget autostart install
gpget autostart status
```

登録できたら、**カメラを一度抜いて、10秒ほど待ってから挿し直します。**

- Windows: 1分以内に取り込みが始まるか、通知が出るか
- Linux: 同上。デスクトップ環境が無い場合、通知は出ずログに記録されます

しばらく待ってから:

```
gpget autostart log
```

何か記録されていれば、その内容を教えてください。**何も起きなくても、
「何も起きなかった」という報告が有用です。**

試し終わったら、元に戻せます。

```
gpget autostart uninstall
```

---

## 4. 問題が起きたときの報告方法

**次の4つを実行して、出力をそのまま貼ってください。**
うまくいかなかったコマンドがあれば、そのエラーメッセージも一緒に。
連絡手段は、直接でも
[Issue フォーム](https://github.com/yager/gpget/issues/new?template=test-report.yml)(英語)
でも構いません。

```
gpget version
gpget probe
gpget autostart status
gpget autostart log
```

あわせて教えていただきたいこと:

- **OS とバージョン**
  - Windows: `winver` の表示、または「設定 > システム > バージョン情報」
  - Linux: `uname -a` と、ディストリ名(`cat /etc/os-release` の先頭)
- **GoPro の機種**
- **何をしたときに、何が起きたか**(期待と違った点)

### ログファイルの場所

`gpget autostart log` が動かない場合は、直接見てください。

| OS | 場所 |
|---|---|
| Windows | `%LOCALAPPDATA%\gpget\autostart.log` |
| Linux | `~/.local/state/gpget/autostart.log` |
| macOS | `~/Library/Logs/gpget-autostart.log` |

### 特に知りたいこと

- **インストールの手順で詰まった箇所**(コマンドが通らない、説明が分からない)
- **警告やセキュリティのダイアログ**が出たか、その文面
- **自動起動が動いたか**。Windows はタスクスケジューラ、Linux は systemd の
  ユーザータイマーを使っています。ここは一度も検証できていません
- **日本語が化けていないか**(通知やメッセージに日本語が含まれます)

---

## 5. 安全性について

- **カメラ側のファイルは読むだけです。**削除・変更・リネームは一切しません
- 保存先に既にあるファイルは、既定では**上書きしません**
- 転送中のファイルは `.part` という一時ファイルとして書かれ、
  完全に受信できてから本来の名前になります。途中で失敗しても
  中途半端なファイルが完成品として残ることはありません
- 外部のサーバーには何も送信しません。通信はカメラとの間だけです

アンインストールは、置いたバイナリを削除するだけです
(`gpget autostart uninstall` を先に実行してください)。設定ファイルは
Windows なら `%APPDATA%\gpget`、Linux なら `~/.config/gpget` にあります。

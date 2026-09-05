#!/usr/bin/env python3
"""GoPro media/list のグループ項目と日付フィールドの仕様を実測で確認する。

読み取り専用。ダウンロードはせず HEAD しか投げない。
FW 更新時や他機種で仕様が変わっていないかの再確認に使う。

    python3 tools/probe_group.py [カメラIP]

確認すること:
  1. グループ項目の `s` が「全フレームの合計バイト」か
     (対立仮説: ファイル数 / 代表1枚のサイズ)
  2. `b`..`l` から `m` を除いた展開が全フレームに到達するか
  3. `media/info` の `ct`(撮影モード)と、`media/list` との `cre` の食い違い

ILS FW H26.03.03.00.00 での既知の結果:
  - s = 合計バイト(公式 SDK の "Number of files in the group" は誤り)
  - t は全モードで "b"、種別は ct(5=burst / 6=timelapse / 10=interval)
  - m は文字列配列・ゼロ詰めなし。削除済みフレームは 404
  - 動画の media/info の cre は UTC(JST と 32400 秒差)。壊れた値を返すこともある
"""
import json
import re
import sys
import urllib.request
from datetime import datetime

CAM = sys.argv[1] if len(sys.argv) > 1 else "172.23.188.51"
BASE = f"http://{CAM}:8080"
CT_KNOWN = {"5": "burst", "6": "timelapse", "10": "interval"}


def fetch(path, method="GET"):
    return urllib.request.urlopen(urllib.request.Request(BASE + path, method=method), timeout=15)


def get_json(path):
    return json.load(fetch(path))


def head_len(path):
    try:
        r = fetch(path, "HEAD")
        return int(r.headers.get("Content-Length", -1)), r.status
    except Exception as e:
        return None, getattr(e, "code", str(e))


def expand(item):
    """b..l から m を除いたフレーム名を返す。"""
    b, l = int(item["b"]), int(item["l"])
    missing = {int(x) for x in (item.get("m") or [])}
    base, ext = item["n"].rsplit(".", 1)
    m = re.search(r"\d+$", base)
    if not m:
        raise ValueError(f"末尾の数字列が見つからない: {item['n']}")
    prefix, width = base[: m.start()], len(m.group())
    return [f"{prefix}{i:0{width}d}.{ext}" for i in range(b, l + 1) if i not in missing], missing


def main():
    ml = get_json("/gopro/media/list")
    print(f"camera={CAM}  session id={ml.get('id')}\n")

    groups = 0
    for block in ml.get("media", []):
        d = block["d"]
        for item in block["fs"]:
            if "g" not in item:
                continue
            groups += 1
            frames, missing = expand(item)
            s = int(item["s"]) if item.get("s", "").isdigit() else None

            print(f"--- {d}/{item['n']}  g={item.get('g')} t={item.get('t')} "
                  f"b={item['b']} l={item['l']} m={sorted(missing) or 'なし'}")
            print(f"    s = {s:,}" if s else "    s = 不明")
            print(f"    展開 {len(frames)} 枚 (範囲 {int(item['l'])-int(item['b'])+1} − 欠番 {len(missing)})")

            total, ok, ng = 0, 0, []
            for fn in frames:
                cl, st = head_len(f"/videos/DCIM/{d}/{fn}")
                if cl and cl > 0:
                    total += cl
                    ok += 1
                else:
                    ng.append((fn, st))
            rep, _ = head_len(f"/videos/DCIM/{d}/{item['n']}")

            print(f"    HEAD 成功 {ok}/{len(frames)}" + (f"  失敗: {ng[:5]}" if ng else ""))
            print(f"    合計バイト = {total:,}")
            print(f"    H1 s=合計バイト : {'OK 一致' if s == total else f'NG 差 {s-total:+,}' if s else '判定不可'}")
            print(f"    H2 s=ファイル数 : {'OK 一致' if s == len(frames) else 'NG 不一致'}")
            print(f"    H3 s=代表1枚    : {'OK 一致' if rep and s == rep else 'NG 不一致'}")

            try:
                ct = get_json(f"/gopro/media/info?path={d}/{item['n']}").get("ct")
                print(f"    ct = {ct} ({CT_KNOWN.get(str(ct), '未知 — 種別不明として扱うこと')})")
            except Exception as e:
                print(f"    ct 取得失敗: {e}")
            print()

    if not groups:
        print("グループ項目がありません。バースト / インターバル / タイムラプスを撮ってから再実行してください。\n")

    # media/list と media/info の cre 食い違い(動画で顕著)
    print("=== cre の食い違い(media/list vs media/info)===")
    fmt = lambda e: datetime.fromtimestamp(int(e)).strftime("%m/%d %H:%M:%S")
    bad = 0
    for block in ml.get("media", []):
        for item in block["fs"]:
            try:
                a = int(item["cre"])
                b = int(get_json(f"/gopro/media/info?path={block['d']}/{item['n']}")["cre"])
            except Exception as e:
                print(f"  {item['n']:<18} info 取得失敗 {e}")
                continue
            if abs(a - b) > 60:
                bad += 1
                print(f"  {item['n']:<18} list={fmt(a)}  info={fmt(b)}  差 {a-b:+,} 秒  ← 要注意")
    print(f"  60秒を超える食い違い: {bad} 件"
          + ("  → 日付には media/list の cre を使うこと" if bad else ""))

    # モード / プリセット構成(参考。転送処理はこれに依存しない)
    print("\n=== モード構成(gpControl)===")
    try:
        gc = get_json("/gp/gpControl")
        names = {m["id"]: m["display_name"] for m in gc.get("modes", [])}
        print(f"  ファーム内部のモード {len(names)} 個(UI 非表示のものも含む):")
        print("    " + ", ".join(f"{i}:{n}" for i, n in sorted(names.items())))
        print("  UI グループ:")
        for g in gc.get("ui_mode_groups", []):
            member = ", ".join(f"{i}:{names.get(i, '?')}" for i in g["modes"])
            print(f"    group {g['id']} → {member}")
        print("  ※ ui_mode_groups は「カメラが持つ構造」。実機の『モードの管理』で"
              "非表示にしていても、ここには出る")
    except Exception as e:
        print(f"  取得失敗: {e}")

    print("\n=== プリセット(presets/get。カスタム含む)===")
    try:
        pr = get_json("/gopro/camera/presets/get")
        cur = get_json("/gopro/camera/state").get("status", {}).get("97")
        for g in pr.get("presetGroupArray", []):
            print(f"  {g.get('id')}:")
            for item in g.get("presetArray", []):
                flags = []
                if item.get("userDefined"):
                    flags.append("ユーザー定義")
                if item.get("isModified"):
                    flags.append("変更済み")
                if item.get("id") == cur:
                    flags.append("← 現在")
                tail = ("  [" + " / ".join(flags) + "]") if flags else ""
                print(f"    {item.get('titleId','?'):<42} {item.get('mode','?')}{tail}")
    except Exception as e:
        print(f"  取得失敗: {e}")


if __name__ == "__main__":
    main()

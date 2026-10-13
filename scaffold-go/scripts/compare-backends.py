"""双后端对照：对 Java 原版与 Go 版发同样的只读请求，比较状态码、响应头和归一化后的 JSON / CSV。

用法（两个后端连同一个库和 Redis）：
  python scripts/compare-backends.py --java http://localhost:8081 --go http://localhost:8080 [--user admin --password admin123]

归一化：去掉 traceId、健康检查的 time；其余字段逐一比较。差异按 JSON 路径打印，并把两边原始响应写入 out/compare/ 便于手工对比。
"""
import argparse
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request

OUT = os.path.join(os.path.dirname(__file__), "..", "out", "compare")


def request(base, method, path, token=None, body=None):
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(base + urllib.parse.quote(path, safe="/?=&%:"), data=data, method=method)
    req.add_header("Content-Type", "application/json")
    if token:
        req.add_header("Authorization", "Bearer " + token)
    try:
        with urllib.request.urlopen(req, timeout=30) as res:
            raw = res.read()
            return res.status, dict(res.headers), raw
    except urllib.error.HTTPError as e:
        return e.code, dict(e.headers), e.read()


def login(base, username, password):
    _, _, raw = request(base, "GET", "/api/auth/captcha")
    cap = json.loads(raw)["data"]
    status, _, raw = request(base, "POST", "/api/auth/login", body={
        "username": username, "password": password, "captcha": cap["code"], "captchaUuid": cap["uuid"],
    })
    body = json.loads(raw)
    if status != 200 or body.get("code") != 200:
        sys.exit(f"login to {base} failed: {status} {raw[:200]!r}")
    return body["data"]["token"]


def normalize(path, headers, raw):
    ctype = headers.get("Content-Type", "")
    if ctype.startswith("application/json"):
        body = json.loads(raw)
        body.pop("traceId", None)
        if path == "/api/health" and isinstance(body.get("data"), dict):
            body["data"].pop("time", None)
        return body
    if ctype.startswith("text/csv"):
        return raw.decode("utf-8")
    return raw


def diff(a, b, prefix="$"):
    """返回差异列表 [(path, java, go)]。"""
    out = []
    # Jackson 把 double 写成 100.0，Go 写成 100；JS 解析后同为 number，按数值比较
    if isinstance(a, (int, float)) and isinstance(b, (int, float)) and not isinstance(a, bool) and not isinstance(b, bool):
        return [] if a == b else [(prefix, a, b)]
    if type(a) != type(b):
        return [(prefix, a, b)]
    if isinstance(a, dict):
        for k in sorted(set(a) | set(b)):
            if k not in a or k not in b:
                out.append((f"{prefix}.{k}", a.get(k, "<missing>"), b.get(k, "<missing>")))
            else:
                out += diff(a[k], b[k], f"{prefix}.{k}")
    elif isinstance(a, list):
        if len(a) != len(b):
            out.append((f"{prefix}.length", len(a), len(b)))
        for i, (x, y) in enumerate(zip(a, b)):
            out += diff(x, y, f"{prefix}[{i}]")
    elif a != b:
        out.append((prefix, a, b))
    return out


def endpoints(java_base, token_java):
    """只读接口清单；带 {id}/{dictType} 的接口从 Java 的列表接口里取真实值展开。"""
    eps = [
        "/api/health",
        "/api/auth/me",
        "/api/auth/routes",
        "/api/dashboard/summary",
        "/api/dashboard/summary?range=7d",
        "/api/dashboard/summary?range=30d",
        "/api/system/permissions/graph",
        "/api/system/users",
        "/api/system/users?pageNum=1&pageSize=2",
        "/api/system/users?keyword=admin",
        "/api/system/users?status=1",
        "/api/system/users?status=0",
        "/api/system/users?deptId=1",
        "/api/system/users?postId=1",
        "/api/system/users/options",
        "/api/system/users/export",
        "/api/system/roles",
        "/api/system/roles?keyword=admin",
        "/api/system/roles?status=1",
        "/api/system/roles/export",
        "/api/system/menus/tree",
        "/api/system/menus/tree?keyword=用户",
        "/api/system/menus/tree?status=1",
        "/api/system/menus/export",
        "/api/system/depts/tree",
        "/api/system/depts/tree?keyword=部",
        "/api/system/depts/export",
        "/api/system/posts",
        "/api/system/posts?keyword=a",
        "/api/system/posts/export",
        "/api/system/dict/types",
        "/api/system/dict/types/export",
        "/api/system/configs",
        "/api/system/configs?keyword=sys",
        "/api/system/configs/export",
        "/api/monitor/oper-logs",
        "/api/monitor/oper-logs?result=0",
        "/api/monitor/oper-logs?keyword=管理",
        "/api/monitor/oper-logs/export",
        "/api/monitor/oper-logs/stats",
        "/api/monitor/oper-logs/stats?range=7d",
        "/api/monitor/oper-logs/stats?range=30d",
        "/api/monitor/oper-logs/timeline",
        "/api/monitor/login-logs",
        "/api/monitor/login-logs?status=0",
        "/api/monitor/login-logs/export",
        "/api/monitor/login-logs/stats",
        "/api/monitor/login-logs/stats?range=7d",
        "/api/monitor/login-logs/heatmap",
        "/api/monitor/login-logs/heatmap?days=30",
        "/api/tool/generator/tables",
        "/api/tool/generator/importable-tables",
        "/api/tool/generator/importable-tables?keyword=sys_",
        # 错误路径
        "/api/system/users?status=abc",
        "/api/system/users/abc",
        "/api/no-such-path",
    ]
    def rows_of(path):
        _, _, raw = request(java_base, "GET", path, token_java)
        data = json.loads(raw).get("data")
        return data["rows"] if isinstance(data, dict) else []  # 低权限账号拿到 403 时跳过展开

    for row in rows_of("/api/system/roles?pageSize=100"):
        eps.append(f"/api/system/roles/{row['id']}/menus")
    for row in rows_of("/api/system/dict/types?pageSize=100"):
        eps.append(f"/api/system/dict/data/{row['dictType']}")
        eps.append(f"/api/system/dict/options/{row['dictType']}")
    for row in rows_of("/api/tool/generator/tables?pageSize=100"):
        eps.append(f"/api/tool/generator/tables/{row['id']}/columns")
        eps.append(f"/api/tool/generator/tables/{row['id']}/preview")
    return eps


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--java", default="http://localhost:8081")
    ap.add_argument("--go", default="http://localhost:8080")
    ap.add_argument("--user", default="admin")
    ap.add_argument("--password", default="admin123")
    ap.add_argument("--anonymous", action="store_true", help="不登录，用于比较 401 行为")
    args = ap.parse_args()
    os.makedirs(OUT, exist_ok=True)

    tj = tg = None
    if not args.anonymous:
        tj = login(args.java, args.user, args.password)
        tg = login(args.go, args.user, args.password)

    eps = endpoints(args.java, tj) if tj else ["/api/health", "/api/auth/me", "/api/system/users", "/api/no-such-path"]
    total = mismatched = 0
    for path in eps:
        total += 1
        sj, hj, rj = request(args.java, "GET", path, tj)
        sg, hg, rg = request(args.go, "GET", path, tg)
        name = path.strip("/").replace("/", "_").replace("?", "_").replace("&", "_").replace("=", "-")
        with open(os.path.join(OUT, f"{name}.java.txt"), "wb") as f:
            f.write(rj)
        with open(os.path.join(OUT, f"{name}.go.txt"), "wb") as f:
            f.write(rg)
        problems = []
        if sj != sg:
            problems.append(("status", sj, sg))
        for h in ("Content-Type", "Content-Disposition"):
            if hj.get(h) != hg.get(h):
                problems.append((f"header {h}", hj.get(h), hg.get(h)))
        try:
            problems += diff(normalize(path, hj, rj), normalize(path, hg, rg))
        except Exception as e:  # 非 JSON 等
            problems.append(("parse", repr(e), ""))
        if problems:
            mismatched += 1
            print(f"\nDIFF {path}  (java {sj} / go {sg})")
            for p, a, b in problems[:25]:
                print(f"   {p}\n      java: {str(a)[:160]}\n      go:   {str(b)[:160]}")
            if len(problems) > 25:
                print(f"   ... {len(problems) - 25} more")
        else:
            print(f"OK   {path}")
    print(f"\n{total} endpoints compared, {mismatched} with differences. Raw responses in {os.path.abspath(OUT)}")
    sys.exit(1 if mismatched else 0)


if __name__ == "__main__":
    main()

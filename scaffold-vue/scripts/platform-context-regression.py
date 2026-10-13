"""应用上下文浏览器回归：自建 A / B 应用并清理，仅用于独立验收环境。

复用 session-regression.py 的登录操作；需要 Python Playwright / Chromium。
覆盖跨页选择、重置 / 导出范围、新增归属、慢请求、账号切换和无应用状态。
"""
import argparse
import asyncio
import importlib
import os
import secrets
from pathlib import Path
from urllib.parse import parse_qs, urlparse

from playwright.async_api import async_playwright, expect

session = importlib.import_module("session-regression")


async def main(base_url, screenshots):
    password = os.environ.get("SCAFFOLD_TEST_PASSWORD", "admin123")
    suffix = secrets.token_hex(5)
    apps, operator_id = [], None
    if screenshots:
        screenshots.mkdir(parents=True, exist_ok=True)
    async with async_playwright() as pw:
        api = await pw.request.new_context(base_url=base_url + "/api/")
        headers = {}
        browser = await pw.chromium.launch(headless=True)

        async def request(method, path, body=None):
            response = await api.fetch(path, method=method, data=body, headers=headers)
            payload = await response.json()
            assert response.ok and payload.get("code") == 200, (method, path, response.status, payload.get("message"))
            return payload.get("data")

        try:
            cap = await request("GET", "auth/captcha")
            auth = await request("POST", "auth/login", {"username": "admin", "password": password,
                                "captcha": cap["code"], "captchaUuid": cap["uuid"]})
            headers["Authorization"] = "Bearer " + auth["token"]
            for letter, key, label, kind in [("A", "balance", "余额", "counter"), ("B", "machineCode", "机器码", "string")]:
                app = await request("POST", "platform/apps", {"code": f"context_{letter.lower()}_{suffix}", "name": f"应用{letter}-{suffix}"})
                app.update(name=f"应用{letter}-{suffix}", letter=letter)
                apps.append(app)
                await request("POST", "platform/fields", {"appId": app["id"], "key": key, "label": label, "type": kind})
                await request("POST", "platform/users", {"appId": app["id"], "username": "context_user", "nickname": f"{letter}应用用户"})
            a, b = apps
            page = await browser.new_page(viewport={"width": 1440, "height": 1000}, reduced_motion="reduce")
            errors, bad = [], []
            page.on("pageerror", lambda error: errors.append(str(error)))
            page.on("response", lambda response: bad.append((urlparse(response.url).path, response.status))
                    if "/api/" in response.url and response.status >= 400 else None)

            async def navigate(label):
                await page.locator(".nav-item").filter(has_text=label).click()
                await expect(page.get_by_role("heading", name=label, exact=True)).to_be_visible()
                await page.wait_for_load_state("networkidle")

            async def select_app(app):
                await page.locator(".app-context").get_by_role("combobox").click()
                await page.get_by_role("option").filter(has_text=app["name"]).click()

            async def current_is(app):
                await expect(page.locator(".app-context").get_by_role("combobox")).to_contain_text(app["name"])

            def belongs_to(response, endpoint, app):
                assert urlparse(response.url).path == "/api/platform/" + endpoint, response.url
                assert parse_qs(urlparse(response.url).query).get("appId") == [str(app["id"])], response.url

            await page.goto(base_url + "/login")
            await session.sign_in(page, "admin", password, "/dashboard")
            await navigate("字段配置")
            await select_app(a)
            await expect(page.get_by_role("cell", name="balance", exact=True)).to_be_visible()
            await navigate("应用用户")
            await current_is(a)
            await expect(page.get_by_text("A应用用户", exact=True)).to_be_visible()
            await expect(page.get_by_role("columnheader", name="余额", exact=True)).to_be_visible()
            await select_app(b)
            await expect(page.get_by_text("B应用用户", exact=True)).to_be_visible()
            await expect(page.get_by_role("columnheader", name="机器码", exact=True)).to_be_visible()
            await navigate("字段配置")
            await current_is(b)

            # 新增 / 编辑字段归属明确，实际请求只写入 B。
            await page.get_by_role("button", name="新增字段", exact=True).click()
            dialog = page.get_by_role("dialog")
            await expect(dialog.get_by_label("所属应用", exact=False)).to_have_value(f"{b['name']}（context_b_{suffix}）")
            assert await dialog.get_by_label("所属应用", exact=False).get_attribute("readonly") is not None
            if screenshots:
                await page.screenshot(path=str(screenshots / "field-ownership.png"))
            await dialog.get_by_placeholder("字母开头", exact=True).fill("contextNote")
            await dialog.get_by_placeholder("表单与列表显示的中文名").fill("回归备注")
            async with page.expect_request(lambda r: r.method == "POST" and urlparse(r.url).path == "/api/platform/fields") as created:
                await dialog.get_by_role("button", name="保存", exact=True).click()
            assert (await created.value).post_data_json["appId"] == b["id"]
            await expect(dialog).not_to_be_visible()
            row = page.get_by_role("row").filter(has_text="contextNote")
            await row.get_by_role("button", name="编辑", exact=True).click()
            await expect(dialog.get_by_label("所属应用", exact=False)).to_have_value(f"{b['name']}（context_b_{suffix}）")
            await dialog.get_by_role("button", name="取消", exact=True).click()
            assert not any(f["key"] == "contextNote" for f in await request("GET", f"platform/fields?appId={a['id']}"))

            await navigate("应用用户")
            await page.get_by_role("button", name="新增用户", exact=True).click()
            await expect(dialog.get_by_label("所属应用", exact=False)).to_have_value(f"{b['name']}（context_b_{suffix}）")
            await dialog.locator('input[autocomplete="off"]').fill("created_" + suffix)
            await dialog.get_by_label("机器码", exact=False).fill("DEVICE-B")
            async with page.expect_request(lambda r: r.method == "POST" and urlparse(r.url).path == "/api/platform/users") as created:
                await dialog.get_by_role("button", name="保存", exact=True).click()
            assert (await created.value).post_data_json["appId"] == b["id"]
            await expect(dialog).not_to_be_visible()
            users = await request("GET", f"platform/users?appId={b['id']}&keyword=created_{suffix}")
            assert users["rows"][0]["profile"]["machineCode"] == "DEVICE-B"
            print("PASS: A 余额 / B 机器码动态列、跨页选择、新增与编辑归属")

            for label, endpoint in [("应用用户", "users"), ("字段配置", "fields"), ("账户流水", "counter-logs"), ("认证日志", "auth-logs")]:
                await navigate(label)
                await current_is(b)
                async with page.expect_response(lambda r: urlparse(r.url).path == "/api/platform/" + endpoint) as reset:
                    await page.get_by_role("button", name="重置", exact=True).click()
                belongs_to(await reset.value, endpoint, b)
                if endpoint != "fields":
                    async with page.expect_download(), page.expect_response(lambda r: urlparse(r.url).path.endswith(endpoint + "/export")) as exported:
                        await page.get_by_role("button", name="导出当前查询结果", exact=True).click()
                    belongs_to(await exported.value, endpoint + "/export", b)
            await page.reload()
            await current_is(b)
            print("PASS: 四页查询 / 重置、三页导出始终带当前 appId，刷新保持选择")

            # 延迟旧应用的字段 / 用户响应，不能覆盖新应用的列或行。
            await navigate("应用用户")
            for endpoint in ["fields", "users"]:
                entered, release = asyncio.Event(), asyncio.Event()
                pattern = f"**/api/platform/{endpoint}?*"

                async def hold_old(route):
                    if parse_qs(urlparse(route.request.url).query).get("appId") == [str(a["id"])]:
                        response = await route.fetch()
                        entered.set()
                        await release.wait()
                        await route.fulfill(response=response)
                    else:
                        await route.continue_()

                await page.route(pattern, hold_old)
                try:
                    await select_app(a)
                    await asyncio.wait_for(entered.wait(), 10)
                    await select_app(b)
                    await expect(page.get_by_text("B应用用户", exact=True)).to_be_visible()
                    release.set()
                    await page.wait_for_load_state("networkidle")
                    await expect(page.get_by_text("A应用用户", exact=True)).not_to_be_visible()
                    await expect(page.get_by_role("columnheader", name="余额", exact=True)).not_to_be_visible()
                    await expect(page.get_by_role("columnheader", name="机器码", exact=True)).to_be_visible()
                finally:
                    release.set()
                    await page.unroute(pattern, hold_old)
            print("PASS: 快速 A → B 切换，旧字段与旧列表晚到均被丢弃")

            for app, button, destination in [(a, "配置字段", "/platform/field"), (b, "管理用户", "/platform/user")]:
                await navigate("应用管理")
                await page.get_by_role("row").filter(has_text=app["name"]).get_by_role("button", name=button, exact=True).click()
                await page.wait_for_url("**" + destination)
                await current_is(app)

            if screenshots:
                await page.screenshot(path=str(screenshots / "app-context-desktop.png"))
                await page.set_viewport_size({"width": 390, "height": 844})
                await page.get_by_role("button", name="外观设置", exact=True).click()
                await page.get_by_role("radio", name="深色", exact=True).click()
                await page.get_by_role("button", name="完成", exact=True).click()
                await page.screenshot(path=str(screenshots / "app-context-mobile.png"))
                assert not await page.evaluate("document.documentElement.scrollWidth > innerWidth")
                await page.goto(base_url + "/platform/app")
                await page.wait_for_load_state("networkidle")
                await page.screenshot(path=str(screenshots / "apps-mobile.png"))
                await page.set_viewport_size({"width": 1440, "height": 1000})
                await navigate("应用用户")

            # 应用列表为空时不向后端发无 appId 的兜底查询，也不能新增或导出。
            async def no_apps(route):
                await route.fulfill(json={"code": 200, "message": "操作成功", "data": []})

            await page.route("**/api/platform/apps/options", no_apps)
            await page.reload()
            await expect(page.locator(".app-context")).to_contain_text("暂无可管理的应用")
            await expect(page.get_by_role("button", name="新增用户", exact=True)).to_be_disabled()
            unscoped = []
            page.on("request", lambda r: unscoped.append(r.url) if "/api/platform/users" in r.url else None)
            for name in ["查询", "重置", "刷新", "导出当前查询结果"]:
                await page.get_by_role("button", name=name, exact=True).click()
            await page.wait_for_load_state("networkidle")
            assert not unscoped, unscoped
            await page.unroute("**/api/platform/apps/options", no_apps)

            roles = await request("GET", "system/roles?pageSize=100")
            role = next(r for r in roles["rows"] if r["code"] == "app_admin")
            username, operator_password = "context_" + suffix, secrets.token_urlsafe(16)
            operator_id = (await request("POST", "system/users", {"username": username, "nickname": "应用上下文回归", "password": operator_password,
                           "roleIds": [role["id"]], "appIds": [a["id"]]}))["id"]
            await session.sign_out(page)
            await session.sign_in(page, username, operator_password, "/dashboard")
            await navigate("字段配置")
            await current_is(a)
            await page.locator(".app-context").get_by_role("combobox").click()
            await expect(page.get_by_role("option").filter(has_text=b["name"])).to_have_count(0)
            await page.keyboard.press("Escape")
            await session.sign_out(page)
            assert not errors, errors
            assert not bad, bad
            print("PASS: 应用管理快捷入口、无应用防误操作、换账号清空应用缓存")
        finally:
            await browser.close()
            if operator_id:
                await request("DELETE", f"system/users/{operator_id}")
            for app in reversed(apps):
                users = await request("GET", f"platform/users?appId={app['id']}&pageSize=100")
                for user in users["rows"]:
                    await request("DELETE", f"platform/users/{user['id']}")
                for field in await request("GET", f"platform/fields?appId={app['id']}"):
                    await request("DELETE", f"platform/fields/{field['id']}")
                await request("DELETE", f"platform/apps/{app['id']}")
            if headers:
                await request("POST", "auth/logout")
            await api.dispose()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--base-url", default="http://localhost:5173")
    parser.add_argument("--screenshots", type=Path)
    args = parser.parse_args()
    asyncio.run(main(args.base_url.rstrip("/"), args.screenshots))

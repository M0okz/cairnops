"""Local fixture-backed alignment audit; never connects to production APIs."""

import argparse
import json
from pathlib import Path
from urllib.parse import urlparse
from playwright.sync_api import sync_playwright
from fixtures import install

parser = argparse.ArgumentParser()
parser.add_argument("--stage", default="before")
parser.add_argument("--url", default="http://127.0.0.1:5199")
parser.add_argument("--widths", default="1920,1280,768,390,320")
parser.add_argument("--locale", choices=["fr", "en"], default="fr")
parser.add_argument("--height", type=int, default=900)
parser.add_argument(
    "--scenario", choices=["normal", "empty", "error", "long"], default="normal"
)
parser.add_argument(
    "--routes",
    default="/,/cibles,/cibles/0,/cibles/rapprochements,/mises-a-jour,/incidents,/maintenance,/connecteurs,/connecteurs/zabbix,/connecteurs/uptime-kuma,/connecteurs/patchmon,/connecteurs/argus,/connecteurs/proxmox,/connecteurs/generic-webhook,/sante,/reglages",
)
args = parser.parse_args()
assert urlparse(args.url).hostname in (
    "localhost",
    "127.0.0.1",
), "Only a local preview is allowed"
out = Path("/tmp/cairnops-alignment-evidence") / args.stage
out.mkdir(parents=True, exist_ok=True)
results = []
with sync_playwright() as p:
    browser = p.chromium.launch(headless=True)
    for theme in ["light", "dark"]:
        for width in map(int, args.widths.split(",")):
            for route in args.routes.split(","):
                context = browser.new_context(
                    viewport={"width": width, "height": args.height},
                    reduced_motion="reduce",
                )
                page = context.new_page()
                errors = []
                page.set_default_timeout(15000)
                page.on("pageerror", lambda e: errors.append(str(e)))
                publish_update = install(page, theme, args.scenario, args.locale)

                try:
                    page.goto(args.url + route)
                    page.wait_for_load_state("networkidle")
                    page.locator(".account-button").wait_for()
                    publish_update()
                    page.evaluate(
                        "document.dispatchEvent(new Event('visibilitychange'))"
                    )
                    page.locator(".update-cta").wait_for()
                    name = route.strip("/").replace("/", "-") or "overview"
                    page.screenshot(
                        path=str(out / f"{name}-{theme}-{width}.png"), full_page=True
                    )
                    geometry = page.evaluate("""() => {
      const box=el=>{const r=el.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height,right:r.right,bottom:r.bottom}};
      const visible=el=>el.checkVisibility({visibilityProperty:true}) && !el.closest('details:not([open])') && !el.closest('.category-tabs, .tabs');
      const spill=[...document.querySelectorAll('main button,main input,main select, main .card,main .row,main .trow')].filter(visible).filter(el=>{const b=box(el);return b.right>innerWidth+1 || b.x< -1}).map(el=>({tag:el.tagName,cls:el.className,text:(el.textContent||'').trim().slice(0,60),...box(el)}));
      return {viewport:innerWidth,scroll:document.documentElement.scrollWidth,update:box(document.querySelector('.update-cta')),updateIcon:box(document.querySelector('.update-cta svg')),instance:box(document.querySelector('.instance-card')),nav:box(document.querySelector('.rail nav')),account:box(document.querySelector('.account-button')),appearance:document.querySelector('.settings-appearance')?box(document.querySelector('.settings-appearance')):null,rename:document.querySelector('.rename')?box(document.querySelector('.rename')):null,spill};
     }""")
                    result = {
                        "route": route,
                        "theme": theme,
                        "width": width,
                        "geometry": geometry,
                        "errors": errors,
                    }
                except Exception as e:
                    result = {
                        "route": route,
                        "theme": theme,
                        "width": width,
                        "errors": errors + [str(e)],
                    }

                if "geometry" in result:
                    g = result["geometry"]
                    u = g["update"]
                    i = g["nav"] if width <= 768 else g["instance"]
                    icon = g["updateIcon"]
                    if g["scroll"] > width + 1 or g["spill"]:
                        result["errors"].append("Horizontal overflow")
                    if width > 1360 or width <= 768:
                        if abs(u["x"] - i["x"]) > 1 or abs(u["right"] - i["right"]) > 1:
                            result["errors"].append("Update and instance edges differ")
                    elif (
                        abs(u["x"] + u["width"] / 2 - (icon["x"] + icon["width"] / 2))
                        > 1
                    ):
                        result["errors"].append("Compact update icon off centre")
                    if width > 768 and g["account"]["bottom"] > args.height:
                        result["errors"].append("Account outside rail viewport")
                    if (
                        g["appearance"]
                        and width > 880
                        and abs(g["appearance"]["right"] - g["rename"]["right"]) > 1
                    ):
                        result["errors"].append("Settings actions misaligned")
                results.append(result)
                print(json.dumps(result), flush=True)
                context.close()
    browser.close()
(out / "geometry.json").write_text(json.dumps(results, indent=2))

raise SystemExit(int(any(row["errors"] for row in results)))

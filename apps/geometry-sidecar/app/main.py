# -*- coding: utf-8 -*-
"""geometry-sidecar：几何约束求解 + SVG 渲染 HTTP 服务。

端点：
  GET  /healthz   健康检查
  POST /solve     单 spec 求解，返回 {coords, report}
  POST /render    单 spec 渲染 SVG（image/svg+xml）
  POST /redraw    求解 + 渲染一步完成：入参可为单 spec，或 {"panels":[spec,…]}
                  多子图格式。每个 panel 各自输出一张独立 SVG。

spec 格式与 recognition-service 的 VLM 提取提示词约定一致。
多子图格式（题目含"图1/图2/图3"等多个几何图时）：
  {
    "panels": [
      {"title": "图1", "points":[...], "anchors":{...}, "constraints":[...],
       "elements":[...], "annotations":[...]},
      {"title": "图2", ...},
      ...
    ]
  }
  每个 panel 用独立的坐标系描述一个子图。
"""
from fastapi import HTTPException, Request
from fastapi.responses import JSONResponse, Response

from .renderer import RenderError, render
from .solver import (GEOM_TOL, ANCHOR_TOL, GeoSolver, SolveError,
                     normalize_constraints, prepare_points)


# 中文字体注册：matplotlib 默认 DejaVu Sans 无 CJK 字形，标题"图1/图2/图3"会显示成方框。
# 启动时探测系统中文字体（Noto Sans CJK / WenQuanYi / PingFang / SimHei / 微软雅黑等），
# 注册到 matplotlib.font_manager，并把字体名塞入 font.sans-serif 优先表。
# Docker 镜像里装 fonts-noto-cjk 后即可被自动探测命中；本机若无，提示 apt install fonts-noto-cjk。
import os as _os  # noqa: E402
import matplotlib  # noqa: E402
import matplotlib.font_manager as _fm  # noqa: E402
from matplotlib import rcParams as _rcParams  # noqa: E402

_CJK_FAMILY_HINT = (
    "Noto Sans CJK SC", "Noto Sans CJK JP", "Noto Sans CJK TC", "Noto Sans CJK",
    "Source Han Sans SC", "Source Han Sans CN",
    "WenQuanYi Zen Hei", "WenQuanYi Micro Hei",
    "PingFang SC", "PingFang TC",
    "Microsoft YaHei", "SimHei", "SimSun", "Heiti SC",
)
_CJK_PATH_HINT = (
    "notosanscjk", "sourcehan", "cjk", "wqy", "zenhei",
    "pingfang", "simhei", "simsun", "msyh", "yahei", "heiti",
)
_CJK_FONT_ROOTS = (
    "/usr/share/fonts", "/usr/local/share/fonts",
    "/opt/fonts",
    _os.path.expanduser("~/.fonts"),
    _os.path.expanduser("~/.local/share/fonts"),
)


def _scan_font_files():
    """扫描常见字体目录，列出所有 ttf/otf/ttc 路径。"""
    out = []
    for root in _CJK_FONT_ROOTS:
        if not _os.path.isdir(root):
            continue
        for dirpath, _dirs, files in _os.walk(root):
            for fn in files:
                if fn.lower().endswith((".ttf", ".otf", ".ttc")):
                    out.append(_os.path.join(dirpath, fn))
    return out


try:
    for _p in _scan_font_files():
        if not isinstance(_p, str):
            continue
        if any(_k in _p.lower() for _k in _CJK_PATH_HINT):
            try:
                _fm.fontManager.addfont(_p)
            except Exception:
                pass

    _loaded = []
    for _f in _fm.fontManager.ttflist:
        _n = getattr(_f, "name", None)
        if isinstance(_n, str):
            _loaded.append(_n)
    _loaded_set = set(_loaded)
    _order = [n for n in _CJK_FAMILY_HINT if n in _loaded_set]

    _existing_raw = _rcParams.get("font.sans-serif", ["DejaVu Sans"])
    _existing = [n for n in _existing_raw if isinstance(n, str)]
    _rcParams["font.sans-serif"] = _order + [n for n in _existing if n not in _order]
    print("[sidecar] CJK fonts registered:", _order)
except Exception as _exc:
    import traceback as _tb  # noqa: E402
    print("[sidecar] CJK font setup failed:", _exc)
    _tb.print_exc()

_rcParams.setdefault("axes.unicode_minus", False)


# FastAPI 会在导入时实例化，但 background 没有 FastAPI 装饰器，直接构造即可。
from fastapi import FastAPI
app = FastAPI(title="geometry-sidecar", version="1.0.0")


@app.get("/healthz")
def healthz():
    return {"status": "ok"}


async def _read_json(request: Request) -> dict:
    try:
        body = await request.json()
    except Exception:
        raise HTTPException(status_code=400, detail="请求体不是合法 JSON")
    if not isinstance(body, dict):
        raise HTTPException(status_code=400, detail="请求体必须是 JSON 对象")
    return body


def _solve_one(spec: dict):
    """单 panel 求解：归一化约束 → GeoSolver → 报告。"""
    points = prepare_points(spec)
    cons, dropped = normalize_constraints(spec.get("constraints"))
    solver = GeoSolver(points, cons, anchors=spec.get("anchors") or {})
    coords, report = solver.solve()
    if dropped:
        report["normalized"] = dropped
    return coords, report


def _has_panels(spec: dict) -> bool:
    return isinstance(spec.get("panels"), list) and len(spec["panels"]) > 0


@app.post("/solve")
async def solve(request: Request):
    """单 spec 求解（向后兼容；多 panel 调用请用 /redraw）。"""
    spec = await _read_json(request)
    try:
        coords, report = _solve_one(spec)
    except (SolveError, RenderError) as e:
        raise HTTPException(status_code=400, detail=str(e))
    return {"coords": coords, "report": report,
            "geom_tol": GEOM_TOL, "anchor_tol": ANCHOR_TOL}


@app.post("/render")
async def render_endpoint(request: Request):
    body = await _read_json(request)
    spec, coords = body.get("spec"), body.get("coords")
    if not isinstance(spec, dict) or not isinstance(coords, dict):
        raise HTTPException(status_code=400, detail="需要 spec 与 coords 字段")
    try:
        svg = render(spec, {k: [float(v[0]), float(v[1])] for k, v in coords.items()})
    except (SolveError, RenderError, KeyError, ValueError, TypeError) as e:
        raise HTTPException(status_code=400, detail=f"渲染失败: {e}")
    return Response(content=svg, media_type="image/svg+xml")


@app.post("/redraw")
async def redraw(request: Request):
    """组合端点：求解 + 渲染一步完成。

    输入可能是：
      - {"panels": [panel, ...]}：一图多子图（如"图1/图2/图3"），逐 panel 独立求解，
        每个 panel 渲染为一张独立 SVG（不拼图）；
      - 单 panel spec（顶层直接含 points/anchors/...）：向后兼容，等价单 panel。
    返回：
      {
        "svgs": [{"title":"图1","svg":"<svg…>"}, ...],
        "report": {max_hard, max_soft, max_all, panels:[{title,max_hard,max_soft}]},
        "geom_tol": ..., "anchor_tol": ...
      }
    """
    body = await _read_json(request)
    try:
        if _has_panels(body):
            panels_specs = body["panels"]
        else:
            panels_specs = [body]

        svgs = []
        panel_reports = []
        max_hard = 0.0
        max_soft = 0.0
        for ps in panels_specs:
            if not isinstance(ps, dict):
                raise SolveError(f"panel 必须是对象，实际为 {type(ps).__name__}")
            coords, report = _solve_one(ps)
            svg = render(ps, coords)
            title = ps.get("title") or ""
            svgs.append({"title": title, "svg": svg.decode("utf-8")})
            max_hard = max(max_hard, float(report.get("max_hard", 0.0)))
            max_soft = max(max_soft, float(report.get("max_soft", 0.0)))
            panel_reports.append({"title": title,
                                  "max_hard": report.get("max_hard", 0.0),
                                  "max_soft": report.get("max_soft", 0.0)})

        merged_report = {
            "max_hard": max_hard,
            "max_soft": max_soft,
            "max_all": max(max_hard, max_soft),
            "panels": panel_reports,
        }
        return JSONResponse({
            "svgs": svgs,
            "report": merged_report,
            "geom_tol": GEOM_TOL,
            "anchor_tol": ANCHOR_TOL,
        })
    except (SolveError, RenderError) as e:
        raise HTTPException(status_code=400, detail=str(e))
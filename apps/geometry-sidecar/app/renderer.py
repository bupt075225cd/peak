# -*- coding: utf-8 -*-
"""几何图渲染器：把约束 spec 与求解坐标画成 SVG。

相比项目外 demo 的增强：
  - 直接输出 SVG（matplotlib savefig format="svg"），矢量、Web 友好；
  - 支持圆（circle）与圆弧（arc）绘图元素；
  - 画布尺寸/边距/标注半径/label_offset 按点集包围盒自适应；
  - 每个子图渲染为一张独立 SVG：/redraw 对 panels 逐张调用本模块，互不拼图；
  - 自动补点：spec 中声明但 elements 未列出的点，自动绘点标记与字母标签。

支持的绘图元素：
  segment/line/ray  {"type":"segment","pts":["A","B"],"style":"solid|dashed","color","lw"}
  circle            {"type":"circle","center":"O","through":"A","color","lw","style"}
  arc               {"type":"arc","center":"O","a":"A","b":"B","color","lw","style"}
  point             {"type":"point","name":"A","color","label_offset":[dx,dy]}

支持的标注：
  angle_mark   {"type":"angle_mark","vertex","from","to","label","color","r","label_gap"}
  right_angle  {"type":"right_angle","vertex","u","v","color","size"}
  text         {"type":"text","xy":[x,y],"text","color","fontsize"}
"""
import io

import numpy as np
import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt  # noqa: E402
from matplotlib.patches import Arc, Circle  # noqa: E402

matplotlib.rcParams["font.sans-serif"] = ["Noto Sans CJK SC", "WenQuanYi Zen Hei",
                                          "Microsoft YaHei", "SimHei", "DejaVu Sans"]
matplotlib.rcParams["axes.unicode_minus"] = False

COLORS = {"black": "#111111", "blue": "#1a5fb4", "gray": "#888888",
          "red": "#c0392b", "green": "#1e8e3e"}


def _c(name):
    return COLORS.get(name, "#111111")


class RenderError(ValueError):
    """渲染失败（如引用了未知点、缺少必要字段）。"""


def _scale_of(coords):
    """参考尺度：demo 的坐标范围约 3 个单位、offset/r 约 0.1~0.16，
    据此把旧 spec 的绝对 offset/r 缩放到当前包围盒尺度，保持视觉比例一致。"""
    xs = np.array([c[0] for c in coords.values()])
    ys = np.array([c[1] for c in coords.values()])
    ex = float(xs.max() - xs.min()) or 1.0
    ey = float(ys.max() - ys.min()) or 1.0
    return max(ex, ey) / 3.0


def _render_into(spec, coords, ax):
    """把单 panel 的几何图画到给定 axes 上。

    spec:  含 points/anchors/constraints/elements/annotations/title
    coords: {name: [x, y]}
    ax:    matplotlib axes
    """
    elements = spec.get("elements") or []
    annotations = spec.get("annotations") or []
    scale = _scale_of(coords)
    s = lambda v: v * scale  # noqa: E731  绝对量按包围盒尺度缩放

    def pt(name):
        if name not in coords:
            raise RenderError(f"元素引用了未定义的点: {name}")
        return np.array(coords[name], float)

    # 1) 圆 / 圆弧（先画，处于线条下层）。
    for el in elements:
        t = el.get("type")
        if t == "circle":
            v, r0 = pt(el["center"]), pt(el["through"])
            r = float(np.linalg.norm(r0 - v))
            ls = (0, (5, 4)) if el.get("style") == "dashed" else "-"
            ax.add_patch(Circle(v, r, fill=False, edgecolor=_c(el.get("color", "black")),
                                lw=el.get("lw", 1.5), ls=ls, zorder=2))
        elif t == "arc":
            v, pa, pb = pt(el["center"]), pt(el["a"]), pt(el["b"])
            r = float(np.linalg.norm(pa - v))
            a1 = np.degrees(np.arctan2(*(pa - v)[::-1]))
            a2 = np.degrees(np.arctan2(*(pb - v)[::-1]))
            sweep = (a2 - a1 + 180) % 360 - 180
            t1, t2 = (a1, a1 + sweep) if sweep > 0 else (a1 + sweep, a1)
            ls = (0, (5, 4)) if el.get("style") == "dashed" else "-"
            ax.add_patch(Arc(v, 2 * r, 2 * r, theta1=t1, theta2=t2,
                             edgecolor=_c(el.get("color", "black")),
                             lw=el.get("lw", 1.5), ls=ls, zorder=2))

    # 2) 线段/射线/直线。
    for el in elements:
        if el.get("type") in ("segment", "line", "ray"):
            a, b = pt(el["pts"][0]), pt(el["pts"][1])
            d = b - a
            if el["type"] == "segment":
                p2 = b
            elif el["type"] == "line":
                a, p2 = a - 0.6 * d, b + 0.6 * d
            else:
                p2 = b + 0.6 * d
            ls = (0, (5, 4)) if el.get("style") == "dashed" else "-"
            ax.plot([a[0], p2[0]], [a[1], p2[1]], color=_c(el.get("color", "black")),
                    lw=el.get("lw", 1.5), ls=ls, zorder=2)

    # 3) 点 + 字母标注。
    point_els = [el for el in elements if el.get("type") == "point"]
    explicit = {el.get("name") for el in point_els}
    auto_points = []
    for name in (list(spec.get("points") or []) + list(coords.keys())):
        if name and name not in explicit and name not in auto_points:
            auto_points.append(name)

    def _draw_point(name, color, label_offset):
        p = pt(name)
        ax.plot(*p, marker="o", ms=3.5, color=color, zorder=3)
        ax.text(p[0] + s(label_offset[0]), p[1] + s(label_offset[1]), name, fontsize=15,
                style="italic", ha="center", va="center", color=color)

    for el in point_els:
        _draw_point(el["name"], _c(el.get("color", "black")), el.get("label_offset") or [0.0, 0.0])
    for name in auto_points:
        _draw_point(name, _c("black"), [0.0, 0.1])

    # 4) 标注：角度弧、直角符号、自由文字。
    for el in annotations:
        t = el.get("type")
        col = _c(el.get("color", "black"))
        if t == "angle_mark":
            v = pt(el["vertex"])
            a1 = np.degrees(np.arctan2(*(pt(el["from"]) - v)[::-1]))
            a2 = np.degrees(np.arctan2(*(pt(el["to"]) - v)[::-1]))
            sweep = (a2 - a1 + 180) % 360 - 180  # 取劣角弧
            r = el.get("r", 0.16 * scale)
            t1, t2 = (a1, a1 + sweep) if sweep > 0 else (a1 + sweep, a1)
            ax.add_patch(Arc(v, 2 * r, 2 * r, theta1=t1, theta2=t2, color=col, lw=1.2))
            if el.get("label"):
                mid = np.radians(a1 + sweep / 2)
                off = r + el.get("label_gap", 0.14 * scale)
                ax.text(v[0] + off * np.cos(mid), v[1] + off * np.sin(mid),
                        el["label"], fontsize=11, color=col, ha="center", va="center")
        elif t == "right_angle":
            v = pt(el["vertex"])
            u, w = pt(el["u"]) - v, pt(el["v"]) - v
            u, w = u / np.linalg.norm(u), w / np.linalg.norm(w)
            sz = el.get("size", 0.08 * scale)
            p1, p2, p3 = v + sz * u, v + sz * u + sz * w, v + sz * w
            ax.plot([p1[0], p2[0], p3[0]], [p1[1], p2[1], p3[1]], color=col, lw=1.1)
        elif t == "text":
            ax.text(el["xy"][0], el["xy"][1], el["text"], fontsize=el.get("fontsize", 11),
                    color=_c(el.get("color", "black")), ha="center", va="center")

    # 5) 画布范围：边距按本 panel 包围盒自适应。
    xs = np.array([c[0] for c in coords.values()])
    ys = np.array([c[1] for c in coords.values()])
    ex = float(xs.max() - xs.min()) or 1.0
    ey = float(ys.max() - ys.min()) or 1.0
    mx, my = ex * 0.10 + 0.1 * scale, ey * 0.18 + 0.1 * scale
    ax.set_xlim(xs.min() - mx, xs.max() + mx)
    ax.set_ylim(ys.min() - my, ys.max() + my)
    ax.set_aspect("equal")
    ax.axis("off")
    if spec.get("title"):
        ax.set_title(spec["title"], fontsize=13)


def render(spec, coords) -> bytes:
    """单个子图渲染为一张独立 SVG。"""
    fig, ax = plt.subplots(figsize=(6.0, 3.2))
    _render_into(spec, coords, ax)
    buf = io.BytesIO()
    fig.savefig(buf, format="svg", bbox_inches="tight", facecolor="white")
    plt.close(fig)
    return buf.getvalue()
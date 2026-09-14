# -*- coding: utf-8 -*-
"""HTTP 层测试：/redraw 对多子图 panels 逐张输出独立 SVG（不拼图）。"""
import json
import os

from fastapi.testclient import TestClient

from app.main import app

DATA = os.path.join(os.path.dirname(__file__), "data")
client = TestClient(app)


def _panel(title, distance):
    return {
        "title": title,
        "points": ["P", "A", "Q"],
        "anchors": {"P": [0, 0], "A": [distance * 0.5, 0], "Q": [distance, 0]},
        "constraints": [
            {"type": "horizontal", "points": ["P", "Q"]},
            {"type": "distance", "a": "P", "b": "Q", "val": distance},
        ],
        "elements": [{"type": "point", "name": n, "label_offset": [0.0, 0.1]} for n in ("P", "A", "Q")],
    }


class TestRedrawMultiSVG:
    def test_single_spec_returns_one_svg(self):
        with open(os.path.join(DATA, "fig1.json"), encoding="utf-8") as f:
            spec = json.load(f)
        r = client.post("/redraw", json=spec)
        assert r.status_code == 200, r.text
        body = r.json()
        assert len(body["svgs"]) == 1
        assert "<svg" in body["svgs"][0]["svg"]

    def test_multi_panels_output_separate_svgs(self):
        """三个子图各自渲染为一张独立 SVG，内容互不相同，不合并成一张。"""
        body = {
            "panels": [_panel("图1", 2.0), _panel("图2", 3.0), _panel("图3", 4.0)],
        }
        r = client.post("/redraw", json=body)
        assert r.status_code == 200, r.text
        data = r.json()
        assert len(data["svgs"]) == 3
        assert [s["title"] for s in data["svgs"]] == ["图1", "图2", "图3"]
        # 三张 SVG 是彼此独立的图（尺寸随坐标范围不同，因而字节长度不同）。
        lengths = {len(s["svg"]) for s in data["svgs"]}
        assert len(lengths) >= 2
        # 报告按各子图独立统计。
        assert len(data["report"]["panels"]) == 3

    def test_legacy_single_panel_reports(self):
        r = client.post("/redraw", json=_panel("图 1", 2.0))
        assert r.status_code == 200, r.text
        data = r.json()
        assert len(data["svgs"]) == 1
        assert data["report"]["panels"][0]["title"] == "图 1"

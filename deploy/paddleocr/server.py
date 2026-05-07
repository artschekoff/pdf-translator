import io
import logging
import os
from contextlib import asynccontextmanager
from typing import Any, Optional

import numpy as np
from fastapi import FastAPI, File, UploadFile
from fastapi.responses import JSONResponse
from PIL import Image
from paddleocr import LayoutDetection, PaddleOCR

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

OCR_LANG = os.environ.get("OCR_LANG", "en")
PADDLE_DEVICE = os.environ.get("PADDLE_DEVICE", "cpu")

_ocr: Optional[PaddleOCR] = None
_layout: Optional[LayoutDetection] = None
_ready = False


def _to_builtin(value: Any) -> Any:
    if isinstance(value, dict):
        return {k: _to_builtin(v) for k, v in value.items()}
    if isinstance(value, list):
        return [_to_builtin(v) for v in value]
    if isinstance(value, tuple):
        return [_to_builtin(v) for v in value]
    if hasattr(value, "item"):
        return value.item()
    return value


def _first_result_json(result_iter: Any) -> dict:
    results = list(result_iter or [])
    if not results:
        return {}
    result = results[0]
    data = getattr(result, "json", {})
    if callable(data):
        data = data()
    data = _to_builtin(data)
    return data if isinstance(data, dict) else {}


def _load_image(data: bytes) -> np.ndarray:
    return np.array(Image.open(io.BytesIO(data)).convert("RGB"))


def _normalize_polygon(points: Any) -> list[list[float]]:
    polygon = []
    for point in points or []:
        if len(point) < 2:
            continue
        polygon.append([float(point[0]), float(point[1])])
    return polygon


def _normalize_coordinate(raw: Any) -> list[float]:
    if len(raw or []) == 4 and not isinstance(raw[0], (list, tuple)):
        return [float(raw[0]), float(raw[1]), float(raw[2]), float(raw[3])]

    xs = []
    ys = []
    for point in raw or []:
        if len(point) < 2:
            continue
        xs.append(float(point[0]))
        ys.append(float(point[1]))
    if not xs or not ys:
        return []
    return [min(xs), min(ys), max(xs), max(ys)]


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _ocr, _layout, _formula_pipeline, _ready
    logger.info("Loading PaddleOCR 3.x models (lang=%s, device=%s)...", OCR_LANG, PADDLE_DEVICE)
    _ocr = PaddleOCR(
        lang=OCR_LANG,
        device=PADDLE_DEVICE,
        enable_mkldnn=False,
        cpu_threads=2,
        text_detection_model_name="PP-OCRv5_mobile_det",
        use_doc_orientation_classify=False,
        use_doc_unwarping=False,
        use_textline_orientation=False,
    )
    _layout = LayoutDetection(
        device=PADDLE_DEVICE,
        enable_mkldnn=False,
        cpu_threads=2,
    )
    _ready = True
    logger.info("PaddleOCR service ready")
    yield


app = FastAPI(lifespan=lifespan)


@app.get("/health")
def health():
    if not _ready:
        return JSONResponse({"status": "loading"}, status_code=503)
    return {"status": "ok"}


@app.post("/ocr")
async def ocr(image: UploadFile = File(...)):
    data = await image.read()
    img_array = _load_image(data)
    result = _first_result_json(_ocr.predict(img_array)).get("res", {})

    texts = result.get("rec_texts") or []
    scores = result.get("rec_scores") or []
    polys = result.get("dt_polys") or result.get("rec_polys") or []

    details = []
    confidences = []
    for idx, text in enumerate(texts):
        if not text:
            continue
        polygon = _normalize_polygon(polys[idx] if idx < len(polys) else [])
        if len(polygon) < 4:
            continue
        confidence = float(scores[idx]) if idx < len(scores) else 0.0
        details.append(
            {
                "text": text,
                "confidence": confidence,
                "bbox": polygon,
            }
        )
        confidences.append(confidence)

    avg_conf = sum(confidences) / len(confidences) if confidences else 0.0

    return JSONResponse(
        {
            "success": True,
            "text": " ".join(texts),
            "confidence": avg_conf,
            "details": details,
        }
    )


@app.post("/layout")
async def layout(image: UploadFile = File(...)):
    data = await image.read()
    img_array = _load_image(data)
    result = _first_result_json(_layout.predict(img_array)).get("res", {})
    raw_regions = result.get("boxes") or []

    regions = []
    label_counts: dict[str, int] = {}
    for raw in raw_regions:
        coordinate = _normalize_coordinate(raw.get("coordinate") or raw.get("bbox") or [])
        if len(coordinate) != 4:
            continue
        label = raw.get("label") or raw.get("type") or "text"
        label_counts[label] = label_counts.get(label, 0) + 1
        regions.append(
            {
                "label": label,
                "score": float(raw.get("score") or raw.get("prob") or 0.0),
                "coordinate": coordinate,
            }
        )
    logger.info("layout labels: %s", label_counts)

    return JSONResponse(
        {
            "success": True,
            "regions": regions,
        }
    )

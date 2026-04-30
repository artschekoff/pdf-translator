import io
import logging
import os
from contextlib import asynccontextmanager
from typing import Optional

import numpy as np
from fastapi import FastAPI, File, UploadFile
from fastapi.responses import JSONResponse
from PIL import Image

from paddleocr import PaddleOCR

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

OCR_LANG = os.environ.get("OCR_LANG", "en")

_ocr: Optional[PaddleOCR] = None
_ready = False


@asynccontextmanager
async def lifespan(app: FastAPI):
    global _ocr, _ready
    logger.info("Loading PaddleOCR model (lang=%s)...", OCR_LANG)
    _ocr = PaddleOCR(use_angle_cls=True, lang=OCR_LANG, use_gpu=False, show_log=False)
    _ready = True
    logger.info("PaddleOCR ready")
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
    img = Image.open(io.BytesIO(data)).convert("RGB")
    img_array = np.array(img)

    result = _ocr.ocr(img_array, cls=True)

    details = []
    texts = []
    confidences = []

    if result and result[0]:
        for line in result[0]:
            bbox, (text, confidence) = line
            details.append({
                "text": text,
                "confidence": float(confidence),
                "bbox": [[float(p[0]), float(p[1])] for p in bbox],
            })
            texts.append(text)
            confidences.append(float(confidence))

    avg_conf = sum(confidences) / len(confidences) if confidences else 0.0

    return JSONResponse({
        "success": True,
        "text": " ".join(texts),
        "confidence": avg_conf,
        "details": details,
    })

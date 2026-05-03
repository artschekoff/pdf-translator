FROM python:3.11-slim

ARG PADDLEPADDLE_VERSION=3.3.1
ARG PADDLEOCR_VERSION=3.5.0
ARG FASTAPI_VERSION=0.136.1
ARG UVICORN_VERSION=0.46.0
ARG PYTHON_MULTIPART_VERSION=0.0.27

ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    PADDLE_PDX_DISABLE_MODEL_SOURCE_CHECK=True

WORKDIR /app

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    libgl1 \
    libglib2.0-0 \
    libgomp1 \
 && rm -rf /var/lib/apt/lists/*

RUN python -m pip install --upgrade pip setuptools wheel && \
    python -m pip install "paddlepaddle==${PADDLEPADDLE_VERSION}" -i https://www.paddlepaddle.org.cn/packages/stable/cpu/ && \
    python -m pip install \
        "paddleocr[doc-parser]==${PADDLEOCR_VERSION}" \
        "fastapi==${FASTAPI_VERSION}" \
        "uvicorn[standard]==${UVICORN_VERSION}" \
        "python-multipart==${PYTHON_MULTIPART_VERSION}"

COPY server.py .

EXPOSE 8000

CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8000"]

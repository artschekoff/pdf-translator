FROM paddlecloud/paddleocr:2.6-cpu-latest

WORKDIR /app

RUN pip install --no-cache-dir paddleocr fastapi "uvicorn[standard]" python-multipart pillow

COPY server.py .

EXPOSE 8000

CMD ["uvicorn", "server:app", "--host", "0.0.0.0", "--port", "8000"]

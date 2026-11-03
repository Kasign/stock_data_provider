FROM python:3.12-slim
ENV PYTHONDONTWRITEBYTECODE=1 PYTHONUNBUFFERED=1 PYTHONPATH=/app
WORKDIR /app
COPY requirements.txt /app/requirements.txt
RUN pip install --no-cache-dir -r /app/requirements.txt
COPY . /app
EXPOSE 8090
CMD ["uvicorn", "python_bridge.capability_app:app", "--host", "0.0.0.0", "--port", "8090", "--no-access-log"]

FROM python:3.12-alpine

WORKDIR /app

# Copy the Python compute logic
COPY logic.py .

# Unbuffered Python stream execution fulfilling the Standard Stream Contract
ENTRYPOINT ["python3", "-u", "/app/logic.py"]

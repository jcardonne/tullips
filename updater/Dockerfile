FROM docker:29-cli
LABEL org.opencontainers.image.source="https://github.com/jcardonne/tullips"
RUN apk add --no-cache python3 postgresql17-client github-cli git kubectl
WORKDIR /opt/updater
COPY update.py adapters.py ./
ENTRYPOINT ["python3", "/opt/updater/update.py"]

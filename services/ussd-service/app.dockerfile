FROM alpine:latest

RUN mkdir /app

COPY app-release /app
COPY templates /app/templates
COPY locales /app/locales
COPY config.yml /app/config.yml
COPY ussd_config.json /app/ussd_config.json

CMD [ "/app/app-release"]
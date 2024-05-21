FROM alpine:latest

RUN mkdir /app

COPY app-release /app
COPY templates /app/templates

CMD [ "/app/app-release"]
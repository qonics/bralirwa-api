FROM alpine:latest

RUN mkdir /app

COPY app-release /app

CMD [ "/app/app-release"]
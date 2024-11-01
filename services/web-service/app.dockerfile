FROM alpine:latest

RUN mkdir /app
RUN mkdir /app/uploads

COPY app-release /app
COPY templates /app/templates
COPY config.yml /app/config.yml
COPY migration /app/migration
COPY assets /app/assets

CMD [ "/app/app-release"]
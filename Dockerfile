FROM alpine:3.9

RUN addgroup -g 1000 -S app && \
    adduser -u 1000 -S app -G app

RUN mkdir /app
COPY dist/i3o /app/i3o

CMD /app/i3o

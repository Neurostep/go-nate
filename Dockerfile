FROM chromedp/headless-shell:91.0.4472.114 as chrome

FROM node:16.3 as node

FROM golang:1.16.5

COPY --from=node /usr/local/bin/node /usr/local/bin/node

COPY --from=chrome /headless-shell /headless-shell

RUN \
    apt-get update -y \
    && apt-get install -y libnspr4 libnss3 libexpat1 libfontconfig1 libuuid1 \
    && apt-get clean && rm -rf /var/lib/apt/lists/* /tmp/* /var/tmp/*

ENV PATH /headless-shell:$PATH

WORKDIR /root/

RUN go install github.com/Neurostep/go-nate@v0.0.7

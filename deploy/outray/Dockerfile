FROM node:24-alpine

ARG OUTRAY_VERSION=0.1.3
RUN npm i -g outray@${OUTRAY_VERSION}

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER node

ENTRYPOINT ["/entrypoint.sh"]

FROM node:24-alpine

ARG OUTRAY_VERSION=0.1.3
RUN npm i -g outray@${OUTRAY_VERSION}

COPY patch-subdomain-conflict.js /patch-subdomain-conflict.js
RUN node /patch-subdomain-conflict.js && rm /patch-subdomain-conflict.js

COPY patch-graceful-shutdown.js /patch-graceful-shutdown.js
RUN node /patch-graceful-shutdown.js && rm /patch-graceful-shutdown.js

COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

USER node

ENTRYPOINT ["/entrypoint.sh"]

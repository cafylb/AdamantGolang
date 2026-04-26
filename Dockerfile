FROM debian:bookworm

COPY run-app /usr/local/bin/
CMD ["run-app"]
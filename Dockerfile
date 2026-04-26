FROM debian:bookworm

COPY run-app /usr/local/bin/
RUN chmod +x /usr/local/bin/run-app
CMD ["run-app"]
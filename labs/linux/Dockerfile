# Image for the Linux course labs. Whatever a student types runs in here, so it
# carries a shell and coreutils and nothing else worth reaching for.
FROM alpine:3.21

RUN apk add --no-cache bash coreutils findutils grep less nano \
    && adduser -D -u 1000 student

USER student
WORKDIR /home/student

# The container has to outlive its own entrypoint: every session attaches with
# docker exec, so PID 1 only needs to sit there until the reaper removes it.
CMD ["sleep", "infinity"]

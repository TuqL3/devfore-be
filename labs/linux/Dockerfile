# Image for the Linux course labs. Whatever a student types runs in here, so it
# carries a shell and coreutils and nothing else worth reaching for.
FROM alpine:3.21

RUN apk add --no-cache bash coreutils findutils grep less nano \
    && adduser -D -u 1000 student

# The home directory is a tmpfs at runtime, so a .bashrc shipped in the image is
# mounted over before the first shell ever reads it. This lives outside home and
# is passed to bash with --rcfile instead.
COPY bashrc /etc/devforge.bashrc

USER student
WORKDIR /home/student

# The container has to outlive its own entrypoint: every session attaches with
# docker exec, so PID 1 only needs to sit there until the reaper removes it.
CMD ["sleep", "infinity"]

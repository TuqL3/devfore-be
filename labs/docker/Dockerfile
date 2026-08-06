# Image for the Docker course labs.
#
# There is no docker daemon in here and there cannot be one: the sandbox runs
# with no network, no capabilities and a read-only root, which is the whole
# reason it is safe to hand a shell to a stranger. The course is graded on the
# Dockerfiles and compose files a student writes, and on the commands they type.
#
# The stub below exists so `docker …` says why rather than "not found", which
# reads as a broken lab. It does not pretend to succeed.
FROM alpine:3.21

RUN apk add --no-cache bash coreutils findutils grep less nano \
    && adduser -D -u 1000 student \
    && printf '%s\n' \
        '#!/bin/sh' \
        'echo "devforge: sandbox không chạy docker daemon — không có mạng, không có quyền." >&2' \
        'echo "Lệnh của bạn vẫn được ghi lại và vẫn được chấm; hãy viết Dockerfile rồi bấm Chấm." >&2' \
        'exit 1' \
        > /usr/local/bin/docker \
    && chmod 755 /usr/local/bin/docker \
    && ln -s /usr/local/bin/docker /usr/local/bin/docker-compose

COPY bashrc /etc/devforge.bashrc

USER student
WORKDIR /home/student

CMD ["sleep", "infinity"]

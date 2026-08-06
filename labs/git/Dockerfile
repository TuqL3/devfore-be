# Image for the Git course labs. The Linux image plus git itself.
#
# Built with `labs/` as the context — see labs/linux/Dockerfile.
FROM alpine:3.21

# `git` pulls in perl-free busybox-compatible defaults; less is git's pager, and
# without it `git log` in a terminal has nothing to page with.
RUN apk add --no-cache bash coreutils findutils grep less nano git \
    && adduser -D -u 1000 student

COPY bashrc /etc/devforge.bashrc

# Every lab is a fresh container, so a repository built in one is gone by the
# next. This rebuilds the starting point in a second — the labs after the first
# are about what you do to a history, not about typing three commits again.
COPY git/kho-mau /usr/local/bin/kho-mau

# There is no network in the sandbox, so every remote a student uses is a bare
# repository they create in their own home. Nothing to configure here beyond
# making sure git does not stop to ask which branch name to use.
RUN chmod 755 /usr/local/bin/kho-mau \
    && git config --system init.defaultBranch main

USER student
WORKDIR /home/student

CMD ["sleep", "infinity"]

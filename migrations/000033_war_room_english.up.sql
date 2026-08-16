-- War Room content is seeded data, so an existing database keeps the Vietnamese
-- rows that scripts/seed.sql inserted before it was translated. The UI is
-- English-only now; this brings the seeded drill in line with it.
--
-- Matched on slug and on order_idx / break_script rather than on the old titles:
-- a database seeded after seed.sql was translated already holds English titles,
-- and this migration must be a no-op there rather than an error.

UPDATE labs SET
    title = 'First Shift',
    description_md = $md$**23:41.** The phone buzzes. The homepage is returning errors and customers are complaining on social media.

That is all you know — exactly like a real shift.

### Your job

The web service runs at `http://127.0.0.1:8080`, and it has a `/healthz` route
that returns exactly the word `ok` when everything is fine:

```sh
curl -i http://127.0.0.1:8080/healthz
```

Make that command answer `ok` again. Then press **Check** — that is also when the
incident clock stops, so do not finish the fix and then sit reading.

### Nobody tells you where it is broken

Deliberately. Finding out where it broke **is** the lesson; knowing up front
turns the rest into typing. Places worth looking first:

```sh
curl -v http://127.0.0.1:8080/healthz   # is it silent, or does it return an error?
ss -ltn                                  # is anything holding port 8080?
ps aux                                   # which processes are running, with which arguments?
ls -l ~/web                              # are the files still there, still readable?
```

The service is a busybox `httpd` serving the `~/web` directory:

```sh
httpd -p 127.0.0.1:8080 -h ~/web
```

> Every command you type in this session is recorded and shown on the results
> page afterwards — so you can see how long you spent down each path. Only you
> and an administrator can read it, and it disappears with the session.$md$
WHERE slug = 'incident-lab-1';

UPDATE lab_tasks SET
    title = 'Restore the service: /healthz on port 8080 returns ok',
    hint = 'Three questions, in that order: is anything listening on port 8080 (ss -ltn), what is the listening process and where does it point (ps aux), is the directory it serves still readable (ls -l ~/web).'
FROM labs l
WHERE lab_tasks.lab_id = l.id AND l.slug = 'incident-lab-1' AND lab_tasks.order_idx = 0;

UPDATE lab_incidents SET
    title = 'The web process died',
    reveal_md = $md$### The `httpd` process is no longer running

Nothing is listening on port 8080, so `curl` reports **Failed to connect** rather
than returning an HTTP status code. That is the cleanest signal of the three
scenarios: the break is at the connection layer, not the application layer.

How to find it: `ss -ltn` shows no line for 8080, `ps aux` shows no `httpd`. Fix
it by starting it again:

```sh
httpd -p 127.0.0.1:8080 -h ~/web
```

In the real world nobody starts it by hand — a process manager (systemd,
supervisor, a container restart policy) brings it back. The real question when
you hit this is **why it died**, and the answer is almost always in the logs or
in the OOM killer.$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script = $sh$pkill httpd$sh$;

UPDATE lab_incidents SET
    title = 'Another process is holding port 8080',
    break_script = $sh$pkill httpd
mkdir -p "$HOME/old-release"
printf 'old release, no healthz\n' > "$HOME/old-release/index.html"
httpd -p 127.0.0.1:8080 -h "$HOME/old-release"$sh$,
    reveal_md = $md$### Port 8080 is taken by a different `httpd`, serving the wrong directory

Something is listening on the port, so `curl` **connects** — it is `/healthz`
that returns **404**. A service answering wrongly is a very different thing from
a service not answering at all, and that is what separates this scenario from
"the process died".

How to find it: `ss -ltn` shows 8080 LISTEN, `ps aux` shows `httpd` running with
`-h /home/student/old-release` — the wrong directory. Fix: kill it and start it
again in the right place.

```sh
pkill httpd
httpd -p 127.0.0.1:8080 -h ~/web
```

In the real world this is a half-finished deploy: the old release never fully
stopped, the new one could not bind the port and died on startup, and what is
serving customers is the release that was supposed to have been replaced. The
lesson: **a port having a listener does not mean the right listener.**$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script LIKE '%old-release%';

UPDATE lab_incidents SET
    title = 'The web directory lost read permission',
    reveal_md = $md$### `~/web` was hit with `chmod 000`

`httpd` is still running and the port is still LISTEN, but it cannot open any
file in the directory, so every path returns **404** — including `/index.html`,
which is still sitting right there.

How to find it: `ls -ld ~/web` shows `d---------`. Fix:

```sh
chmod 755 ~/web
```

The easiest way to waste time on this scenario is trusting the status code: 404
reads as "the file does not exist", so people go looking for the file before
looking at permissions — and the file is still there. In the real world this
tends to follow a `chmod`/`chown` run against the wrong directory, or a deploy
process running as a different user.$md$
FROM labs l
WHERE lab_incidents.lab_id = l.id AND l.slug = 'incident-lab-1'
  AND lab_incidents.break_script LIKE 'chmod 000%';

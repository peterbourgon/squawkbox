# squawkbox

This is an HTTP server that's designed to be hooked up to Twilio. Specifically,
it's meant to take calls from an intercom system at the front of an apartment
building, and provide value-add services, like auto-bypass codes, or saving and
hosting of call recordings. A work in progress.

Prior art: [tessr/doorbell](https://github.com/tessr/doorbell).

```
$ squawkbox -h
Usage of squawkbox:
  -addr string
        listen address (default ":6175")
  -auth string
        file containing HTTP Basic Auth user:pass
  -bypass string
        file containing secret bypass code (optional)
  -forward string
        file containing forwarding phone number
  -recordings string
        path to save recordings
```

Secrets are kept in files for security purposes.

```
echo "username:password" > basic_auth.txt
chmod 600 basic_auth.txt
echo "123456" > bypass_code.txt
chmod 600 bypass_code.txt
echo "2125551212" > forward_number.txt
chmod 600 forward_number.txt
mkdir recordings/

squawkbox \
  -auth       basic_auth.txt     \
  -bypass     bypass_code.txt    \
  -forward    forward_number.txt \
  -recordings recordings/
```
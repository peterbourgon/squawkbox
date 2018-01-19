# squawkbox

This is an HTTP server that's designed to be hooked up to Twilio. Specifically,
it's meant to take calls from an intercom system at the front of an apartment
building, and provide value-add services, like auto-bypass codes, or saving and
hosting of call recordings. A work in progress.

Prior art: [tessr/doorbell](https://github.com/tessr/doorbell).

```
USAGE
  squawkbox [flags]

FLAGS
  -addr 127.0.0.1:9176                                  listen address
  -authfile ...                                         file containing HTTP BasicAuth user:pass:realm
  -codesfile codes.dat                                  file to store bypass codes
  -debug false                                          debug logging
  -eventsfile events.dat                                file to store audit events
  -forward Connecting you now.                          forward text
  -forwardfile ...                                      file containing number to forward to
  -greeting Hello; enter code, or wait for connection.  greeting text
  -noresponse Nobody picked up. Goodbye!                no response text
  -recordingsdir recordings                             directory containing saved recordings

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

# ctx API

## ctx.now()
Returns the current instant as a JavaScript `Date` (local timezone).

## ctx.today()
Returns today's date as a `"YYYY-MM-DD"` string.

## ctx.weekday()
Returns the lowercase weekday name (e.g. `"monday"`).

## ctx.dayOfMonth()
Returns the day of the month as an integer 1..31.

## ctx.month()
Returns the month as an integer 1..12.

## ctx.year()
Returns the four-digit year as an integer.

## ctx.isFirstOfMonth()
Returns `true` when today is the 1st of the month.

## ctx.isLastOfMonth()
Returns `true` when today is the last day of the month.

## ctx.isWeekday(name)
Returns `true` when today matches the lowercase weekday name passed in.

## ctx.daysSince(dateOrString)
Returns the integer number of days since the given `Date` or `"YYYY-MM-DD"` string (negative if it's in the future).

## ctx.daysBetween(a, b)
Returns the integer number of days between two dates or `"YYYY-MM-DD"` strings.

## ctx.addDays(date, n)
Returns a new `Date` `n` days after the input.

## ctx.formatDate(date)
Formats a `Date` as a `"YYYY-MM-DD"` string.

## ctx.parseDate("YYYY-MM-DD")
Parses a `"YYYY-MM-DD"` string into a `Date`.

## ctx.script.id
Numeric id of the running script.

## ctx.script.name
Current display name of the running script.

## ctx.script.trigger
How this run was triggered: `"scheduled"` or `"manual"`.

## ctx.script.lastRunAt
The previous run's start time as a `Date`, or `null` if this is the first run.

## ctx.lastSpawn
The most recent `Task` spawned by this script, or `null`. Shape: `{ id, title, notes, state, due_date, created_at, completed_at, cancelled_at, tags: [...] }`.

## ctx.state.get(key)
Reads a persistent value from this script's state blob.

## ctx.state.set(key, value)
Buffers a write to this script's state blob. Flushed only on successful run end.

## ctx.state.delete(key)
Buffers a delete of a key in this script's state blob.

## ctx.state.all()
Returns a snapshot of the entire state object (after buffered writes in this run).

## ctx.log(msg)
Writes an info-level log line. Equivalent to `ctx.log.info(msg)`.

## ctx.log.debug(msg)
Writes a debug-level log line.

## ctx.log.info(msg)
Writes an info-level log line.

## ctx.log.warn(msg)
Writes a warn-level log line.

## ctx.log.error(msg)
Writes an error-level log line.

## console.log / info / warn / error
Aliases for the matching `ctx.log[level]` methods.

## ctx.queueTask({ title, notes?, tags?, due_date? })
Queues a task to be created. Validated immediately; persisted only if the run finishes successfully. Tag names that don't exist yet are auto-created.

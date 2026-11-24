# Holiday meals

A meal in Sous is several recipes served together: Thanksgiving, a birthday
dinner, Sunday lunch. Sous works out when to do every step so everything is
ready when it's served, without asking one oven to be two temperatures or
one cook to be in two places, and then keeps time with you on the day.

## 1. Describe your kitchen

Open **Meals**, then **Your kitchen**:

- **Ovens** and how many dishes each holds at once (racks). An oven holds one
  temperature at a time; dishes within 25°F of each other share it.
- **Burners** on the stovetop.
- **Other appliances**: a slow cooker, a grill, a pressure cooker. Name them
  as recipe steps do, since steps are matched to them by name.
- **Serving dishes**, so a meal can check every dish has something to go to
  the table in.
- **The kitchen's day**: hands-on work is planned between these hours when it
  can be. Things that look after themselves (a brine, an overnight chill) can
  run outside them.

You only do this once; every meal uses it.

## 2. Give the recipes timing

Sous plans better when steps say how long they take and what they need. In
the recipe editor, each step has **Timing for meal plans** beneath it:

- **Minutes**, and whether it **needs the cook the whole time** (chopping,
  stirring) or looks after itself (roasting, resting).
- What it **uses**: the oven (and at what temperature, on how many racks),
  the stovetop (how many burners), or an appliance.
- **Can wait after**: the dish can stop after this step and wait this long,
  in the fridge, on the counter or somewhere warm. That makes the steps up
  to here **make-ahead**, which the plan moves earlier, even days before. A
  reheat time (and oven temperature) adds a reheat on the day.

Opening a step's timing shows what Sous reads in its words ("Sous reads:
45 min · oven 350°F · hands off"), and **Use it** fills it in. Below the
steps, **Fill them from Sous's reading** does that for every step without
timing at once, to check before saving. Steps you haven't timed are guessed
the same way when a meal is planned, and the plan says which dishes it
guessed for.

Sous's reading is a pattern-match on the words, and it often gets wrong
what needs the cook and what can be made ahead. A connected AI assistant
reads recipes properly and can fill in timing for the whole library, or
just one meal's dishes, without changing any step's words. See
[Connecting assistants](agents.md#filling-in-step-timing), or **Assistants**
in Sous, under **Fill in step timing**.

## 3. Plan the meal

Under **Meals**, name the meal and when it's served, then add the dishes from
your library. For each dish you can set:

- **When it's served**: with the meal, before it (an appetiser), or later
  (dessert an hour and a half after, say).
- **Its serving dish**, from your kitchen's list.

Set how many **cooks** there are (how many hands-on jobs can happen at once)
and how early **prep** may start (the day itself, the day before, or more).

The plan appears beside the menu, day by day. Each task shows when it starts,
how long it takes, which oven at what temperature, and whether it's
hands-on. Make-ahead dishes say where they wait and until when. Below the
plan are each oven's day and what goes to the table in what.

At the top, a strip of lanes shows the meal's day at a glance: what each
oven, the stovetop and your hands are doing hour by hour, with solid bars
for hands-on work and hatched ones for things that look after themselves.

If the meal is running late, the header says when it now lands, which dish
is holding it up, and what's in its way ("the oven is busy with the pie at
400°F until 12:20 AM"). Sous works out what would help, planning each in
full: another cook, the meal without that dish, or the meal served later
(every dish moves with it, so nothing is rushed). The one that brings the
meal in soonest comes first, and only options that gain real time are
offered. A later time past your kitchen's hours says so. If that dish's
make-ahead didn't happen, you can start it now or mark it already made.
Serving it on its own later, and any other late dishes (which don't change
when the meal lands), are under **Other fixes**. Every fix can be undone
from the message that follows. **To the table** shows when each dish will
really be ready, and the Meals list shows how late today's meal is. On the
day the page keeps itself up to date.

Warnings say what else to check: a dish that would sit waiting because the
kitchen is busy, work that has to start before your prep window, or a
serving dish planned for two things at once. Tap one to jump to the dish or
step it's about. **Pin** a task to keep it where it is while the rest of the
plan moves around it. Changing the meal's time, cooks or prep says how many
steps moved, with Undo.

At a keyboard, **C** cooks the meal and **D** adds a dish (the arrow keys
pick from the matches); **?** lists the keys. In the lanes, Tab reaches the
chart once and the arrow keys move between its bars.

## 4. Cook it

On the day, open the meal and press **Cook the meal**. The screen stays on.

- What to do now hangs from the rail: one ticket per cook, the most urgent
  first. **Start** begins a task's countdown; **Done** ticks it off, and
  the message it leaves has Undo. Before a task starts, **Already done**
  ticks it off without the countdown. A dish's next step only comes up once
  the one before it is done.
- Sous doesn't assume an oven is on. When one needs heating, a ticket says
  **Turn the oven on to 350°F**; press **It's on** and it counts down until
  it's hot.
- Things that look after themselves (a roast, an oven heating) count down
  in a lane beside it, and turn red when they're due out.
- **Up next** shows what's coming and when, led by anything due that's
  waiting for free hands. **Ovens** shows what each oven is doing.
- If make-ahead work didn't happen (the pie meant for yesterday), it's
  listed above the rail: **Start it now** puts it on the rail, **It's
  already made** ticks it off.
- If you fall behind, Sous replans from now, giving the oven and your
  hands to the dish in most trouble first, and the bar says when the meal
  now lands. Tap it for the same fixes as the meal's page. A step moved
  later says "Starts in 4 min" until its time, then "Start now · was 7:40".
- When another cook ticks something off on their device, a message says
  who and what. Coming back to the screen after a while, a message says
  what came up while you were away.
- **The recipe at step N** jumps into that recipe's own cook mode.
- **Done** keeps what you've finished, with Undo. **Start over** forgets the
  progress (pins stay) to cook the meal again.

Several people can cook from several devices: each one's taps reach the
others within about 20 seconds.

## Things it doesn't do yet

- Fridge space, and a warm hold that ties up an oven.
- Steps inside one recipe happening side by side: a recipe's steps run in
  order (but different dishes overlap freely).
- Dragging a task to a new time; pinning keeps it where the plan put it.
- Scaling recipes up for the number of guests.
- Meals need a connection to Sous; they aren't kept for offline use.

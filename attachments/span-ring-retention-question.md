# I7: span-ring retention needs a separate argument

This is a scoped proof question under live I7 request42c2ce37/promise5cfedb59,
not an evaluator leak claim, an adopted profile change, or a completion report.
The private evaluator is clean at f7e6f4fa58073b010fed88a51f4f7ed0f6fed842.
The supported source profile is ordinary Go1.26.7 on Darwin/Linux ARM64/AMD64;
GreenTeaGC is enabled by the toolchain default. The current disposable P1
provider/session fixture was built for all four targets and run only natively.
The issue below concerns a runtime component needed by the general theorem;
the fixture's fixed seven programs have not been shown to produce this trace.

## Source facts

In runtime/mgcmark_greenteagc.go, spanQueue.drain (:460-508) grows a chain when
its current head has insufficient room. Capacity doubles from at least1024
entries (physical-page rounding can increase this) through a maximum131072.
Once the maximum is reached, a full head can still be replaced by another
maximum-capacity ring. newSpanSPMC (:708-729) obtains each backing with sysAlloc.
Each maximum ring requests1048576 bytes, outside the ordinary page heap.

spanQueue.steal (:543-581), including q==q2, removes an empty old tail from the
active chain and sets its dead flag. The descriptor and ring remain on the
separate work.spanSPMCs list. freeDeadSpanSPMCs (:756-792) takes its lock and
refuses unless gcphase is _GCoff. When admitted, it walks the complete list,
frees dead ring mappings and returns their descriptors to fixalloc. A finite
active queue depth is consequently not itself a finite dead-list history.

The complete source call inventory has two callers of freeDeadSpanSPMCs:
bgsweep (mgcsweep.go:310), after its sweep and work-buffer loops; and the
synchronous-sweep branch of gcSweep (mgc.go:2084). gcStart selects that latter
mode only with debug.gcstoptheworld==2 (mgc.go:782-787), which is not the adopted
ordinary default. P destruction has a separate spanQueue.destroy call at
proc.go:5974; the current fixed-P fixture does not destroy its P.

runtime.GC (mgc.go:522-591) waits for marking and helps/waits for sweeping,
then publishes profiling state. It does not directly call freeDeadSpanSPMCs.
This does NOT prove that bgsweep can be postponed across arbitrary completed
GC calls: its scheduling and the queue population may imply an additional
bound. That implication needs an explicit proof. The accepted completed-sweep
cut alone, or assuming dead rings are ordinary collectible heap objects,
does not discharge this component.

## Small count model and its limits

The attached Python model implements the source's local256 ring, spill64/16,
drain128, capacity1024..131072, consumer refill and dead-tail transition using
integer counts instead of allocating Go objects or ring memory. Every modeled
cycle queues the same393472 items, drains everything and ends with one empty
active maximum ring. Without the separate cleanup transition, dead retained
rings grow from9 after the first cycle to42 after twelve. With _GCoff cleanup
after each cycle, each cut retains exactly one1048576-byte ring. A cleanup
attempt during marking preserves the dead list, matching the phase guard.

This is an omission-sensitive check of a proposed accounting inference. It
omits the actual scheduler, collector transitions, span-ownership graph and
admitted evaluator inputs. It is not a realizable native trace, a measurement
of the fixture, or proof of unbounded storage in an admitted execution.

There is a second sufficient route: if the peak number of outstanding queued
span entries for a P can be proved <=131072, its head eventually has enough
capacity for every later drain. A failed drain requires head occupancy plus
those local entries to exceed head capacity, so a head at the peak bound
cannot fail. With no P destruction, at most eight geometrically increasing
rings per P are then ever created, independent of cleanup. Pinned source
spanInlineMarkBits ownership prevents two queued representatives for the same
span; a span currently being scanned may independently have one queued
representative after release. This may allow an eligible-span census tighter
than total reserved pages. No such complete <=131072 census is claimed here.
If the proven peak exceeds the cap, this finite-growth argument does not apply.

## Independent question

Determine whether the existing adopted four-build conditions already imply a
finite retained ring bound, either through an explicit bgsweep/phase-progress
argument, a sufficient eligible-span population bound, or another source
invariant. Preserve the full admitted language and cumulative W/A/D contract.
Do not silently change Go experiments, GODEBUG, collector behavior, host
premises or budget limits. If neither route follows, identify the precise
missing premise and the smallest concrete correction to take to the governing
author; do not report full I7 infeasibility from this model alone.

The implementing promise remains live. Other independent runtime components
and the current evaluator evidence remain available; full F, core delivery,
independent implementation review and publication gates remain unpassed.

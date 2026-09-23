# Atomic Authorize Spec

## Overview 
- Currently authorize_payment command can run into concurrencies issues because it first retrieves and order and then it makes provider calls, only after then it updates the state. Another thread can interleve in between. 

## In Scope
- Solve it by claiming the order by trainsitioning it into an intermediary state. This is going to solve the big chunk of the race condition but not all. We still need atomic get and update in the same transaction. Add a new function to the interface with 
SaveIfStateIs(state, order) error. This is going to be used to transition the order from the initialized state to authorizing state. The state param is the state the stored copy must still be in for the write to happen; on a mismatch it returns models.ErrStateChanged, which the handler maps to 409. The event that enters the new state is entering_authorization, and like every other transition it is recorded in history. 

## Out Scope
- Recovering an order stuck in authorizing because its process died mid-authorization. No sweeper, no timeout, no reconciliation against the provider. 
- complete_order, which still does an unguarded read-modify-write. 

## Acceptance critera
- Getting and order and Transitioning to authorizing state must be atomic and cannot be interleaved with another thread. 
- Now we are adding a new state that will impact transition into and out of 3 states: initialized, payment_authorized, rejected. No other state changes behaviour. Their tests do change where a test enumerates every event, since the new event has to be listed there too. 
- If there are tests that depend on number of states, or other state transitions, they need to be updated. 
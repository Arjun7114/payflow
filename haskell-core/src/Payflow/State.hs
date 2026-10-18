{-# OPTIONS_GHC -Wincomplete-patterns #-}

-- | The pure payment state machine, ported from payflow's Go domain core.
--
-- The point of this port is what the type system makes /impossible/. A
-- 'PaymentState' is a sum type: a value can only ever be one of the eight
-- listed states — there is no way to construct an invalid one. The 'transition'
-- function is total over (state, command) pairs, and because the library is
-- built with -Wincomplete-patterns, GHC refuses to compile if any case is left
-- unhandled. Illegal transitions are represented by returning 'Nothing'; the
-- set of legal transitions is the nine 'Just' cases below, exactly mirroring
-- the Go transition table.
module Payflow.State
  ( PaymentState (..)
  , Command (..)
  , Event (..)
  , transition
  , isTerminal
  , allStates
  , allCommands
  ) where

-- | Every state a payment can occupy. A sum type, so an invalid state is
-- unrepresentable — the Go version could only approximate this with a string
-- and a runtime check.
data PaymentState
  = Initiated
  | Authorized
  | Captured
  | Settled
  | Refunded
  | Failed
  | Expired
  | FlaggedHold
  deriving (Eq, Show, Enum, Bounded)

-- | An intent to change a payment's state.
data Command
  = Authorize
  | Capture
  | Settle
  | Refund
  | Expire
  | Fail
  | FlagHold
  | ClearHold
  deriving (Eq, Show, Enum, Bounded)

-- | A fact recorded when a command succeeds.
data Event
  = Authorized_
  | Captured_
  | Settled_
  | Refunded_
  | Failed_
  | Expired_
  | HoldFlagged_
  | HoldCleared_
  deriving (Eq, Show)

-- | The whole state machine. A legal (state, command) pair yields
-- @Just (nextState, event)@; every illegal pair yields @Nothing@. These nine
-- 'Just' clauses are exactly the nine legal edges in payflow's lifecycle
-- diagram. The final catch-all makes the function total and rejects all other
-- pairs — the equivalent of the Go table's "absent entry means illegal".
transition :: PaymentState -> Command -> Maybe (PaymentState, Event)
transition Initiated   Authorize = Just (Authorized,  Authorized_)
transition Initiated   Fail      = Just (Failed,      Failed_)
transition Authorized  Capture   = Just (Captured,    Captured_)
transition Authorized  Expire    = Just (Expired,     Expired_)
transition Authorized  FlagHold  = Just (FlaggedHold, HoldFlagged_)
transition FlaggedHold ClearHold = Just (Captured,    HoldCleared_)
transition Captured    Settle    = Just (Settled,     Settled_)
transition Captured    Refund    = Just (Refunded,    Refunded_)
transition Settled     Refund    = Just (Refunded,    Refunded_)
transition _           _         = Nothing

-- | A terminal state has no legal outgoing transition.
isTerminal :: PaymentState -> Bool
isTerminal s = all (\c -> transition s c == Nothing) allCommands

-- | Every state, derived automatically from the type (Enum/Bounded).
allStates :: [PaymentState]
allStates = [minBound .. maxBound]

-- | Every command, derived automatically from the type.
allCommands :: [Command]
allCommands = [minBound .. maxBound]
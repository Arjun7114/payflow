{-# LANGUAGE OverloadedStrings #-}
{-# LANGUAGE ScopedTypeVariables #-}

-- | Property tests for the pure payment state machine, mirroring the Go
-- invariants. Hedgehog generates random states, commands, and command
-- histories and checks that the laws hold across all of them.
module Main (main) where

import Control.Monad (unless)
import System.Exit (exitFailure)

import Hedgehog (Property, property, forAll, assert, (===), checkParallel, Group (..), Gen)
import qualified Hedgehog.Gen as Gen
import qualified Hedgehog.Range as Range

import Payflow.State

-- | The nine legal (state, command) pairs, written out independently so the
-- test does not simply re-import the transition function it is checking. If the
-- transition table changes, this list must change to match — the two are
-- cross-checked by the property below.
legalEdges :: [(PaymentState, Command, PaymentState)]
legalEdges =
  [ (Initiated,   Authorize, Authorized)
  , (Initiated,   Fail,      Failed)
  , (Authorized,  Capture,   Captured)
  , (Authorized,  Expire,    Expired)
  , (Authorized,  FlagHold,  FlaggedHold)
  , (FlaggedHold, ClearHold, Captured)
  , (Captured,    Settle,    Settled)
  , (Captured,    Refund,    Refunded)
  , (Settled,     Refund,    Refunded)
  ]

isLegal :: PaymentState -> Command -> Bool
isLegal s c = any (\(f, cmd, _) -> f == s && cmd == c) legalEdges

genState :: Gen PaymentState
genState = Gen.element allStates

genCommand :: Gen Command
genCommand = Gen.element allCommands

-- Property 1 — exhaustive legality. For every (state, command), transition
-- agrees with the declared legal-edge set: legal pairs yield Just, all others
-- yield Nothing.
prop_matchesLegalSet :: Property
prop_matchesLegalSet = property $ do
  s <- forAll genState
  c <- forAll genCommand
  case transition s c of
    Just _  -> assert (isLegal s c)
    Nothing -> assert (not (isLegal s c))

-- Property 2 — terminal states are final. No command transitions out of a
-- terminal state (failed, expired, refunded).
-- Property 2 — terminal states are final. No command transitions out of a
-- terminal state (failed, expired, refunded). We generate only the terminal
-- states directly, so every generated case is meaningful (no discards).
prop_terminalFinal :: Property
prop_terminalFinal = property $ do
  let terminals = filter isTerminal allStates
  s <- forAll (Gen.element terminals)
  c <- forAll genCommand
  transition s c === Nothing

-- Property 3 — no double capture. Walk a random command history from
-- Initiated; a payment is never captured more than once.
prop_noDoubleCapture :: Property
prop_noDoubleCapture = property $ do
  cmds <- forAll (Gen.list (Range.linear 0 20) genCommand)
  let (_, captures) = foldl step (Initiated, 0 :: Int) cmds
      step (st, n) c = case transition st c of
        Just (next, _)
          | next == Captured && st /= Captured -> (next, n + 1)
          | otherwise                          -> (next, n)
        Nothing -> (st, n)
  assert (captures <= 1)

main :: IO ()
main = do
  ok <- checkParallel $ Group "Payflow.State"
    [ ("transition matches legal set", prop_matchesLegalSet)
    , ("terminal states are final",    prop_terminalFinal)
    , ("no double capture",            prop_noDoubleCapture)
    ]
  unless ok exitFailure
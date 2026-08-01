// Package helpers contains shared test fixtures and helpers for the e2e
// test suite.
package helpers

// SultansTab is a real-world ASCII tab used across multiple e2e tests.
const SultansTab = `Dire Straits
Sultans of Swing
Tuning: E Standard

e|---------------------------------|-----------------|
B|---3---3---2---0---0---0---3---0-|---1---0---------|
G|---------------------------------|-----------------|
D|---------------------------------|-----------------|
A|---------------------------------|-----------------|
E|---------------------------------|-----------------|
`

// DropDTab is a simple open-chord tab in Drop D tuning.
const DropDTab = `Foo Fighters
Everlong
Tuning: Drop D

e|---0---0---0---0---|
B|---0---0---0---0---|
G|---0---0---0---0---|
D|---0---0---0---0---|
A|---0---0---0---0---|
D|---0---0---0---0---|
`

// FreeformTab has no bar delimiters.
const FreeformTab = `Freeform

e|--0--3--5--7--3--0--|
B|--------------------|
G|--------------------|
D|--------------------|
A|--------------------|
E|--------------------|
`

// MultiDigitTab tests fret numbers > 9.
const MultiDigitTab = `Multi-digit

e|------12-------|
B|---10----10----|
G|--------------9|
D|--------------9|
A|--------------7|
E|--------------0|
`

// SmokeTab is a Drop D power-chord riff used for MIDI tests.
const SmokeTab = `Smoke on the Water
Deep Purple
Tuning: Drop D

D|---0---3---5---0---0---3---6---5---|
A|---0---3---5---0---0---3---6---5---|
D|---0---3---5---0---0---3---6---5---|
`

// MonoTab is a single-string line used for timing tests.
const MonoTab = `Mono
Tuning: E Standard

E|----0-----3-----5---|
`

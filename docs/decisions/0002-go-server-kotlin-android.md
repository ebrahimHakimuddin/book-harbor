# 0002: Use Go for the server and Kotlin for Android

- Status: accepted
- Date: 2026-09-21

## Decision

BookHarbor will begin with a Go server and a native Kotlin Android application
using Jetpack Compose. The two applications communicate only through the
versioned HTTP interface.

## Reason

A Go server can be distributed as a small binary or container with few runtime
requirements, which suits self-hosting on modest hardware. Kotlin provides
first-class access to Android platform features needed for durable downloads,
offline storage, background synchronization, and native reading integrations.

Keeping the implementations separate makes the HTTP interface a real seam. It
also avoids coupling either application to a cross-platform abstraction before
the EPUB and PDF reader requirements are understood.

## Consequences

- Domain types are not shared as source code between server and Android.
- The versioned contract and compatibility tests must prevent schema drift.
- Deployment remains independent of the Android release cycle.
- A future web administration interface can use a separate frontend stack
  without changing the server's domain modules.

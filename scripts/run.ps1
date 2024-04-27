#!/usr/bin/env pwsh

Write-Host "Running localtest!"

if (Get-Command "docker" -errorAction SilentlyContinue)
{
    Write-host "Running using docker"
    docker compose --profile "*" down -v
    docker compose --profile "*" up -d --build
}
elseif (Get-Command "podman" -errorAction SilentlyContinue)
{
    Write-host "Running using podman"
    # TODO implement
}
else
{
    Write-host "Neither Docker or Podman was found"
}


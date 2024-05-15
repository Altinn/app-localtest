#!/usr/bin/env pwsh

if ($args[0] -eq "stop")
{
    Write-Host "Stopping localtest!"

    if (Get-Command "docker" -errorAction SilentlyContinue)
    {
        Write-host "Stopping using docker"
        docker compose --profile "*" down -v
    }
    elseif (Get-Command "podman" -errorAction SilentlyContinue)
    {
        Write-host "Stopping using podman"
        podman compose --file podman-compose.yml --profile "*" down -v
    }
    else
    {
        Write-host "Preqreqs missing - please install docker or podman"
    }
}
else 
{
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
        podman compose --file podman-compose.yml --profile "*" down -v
        podman compose --file podman-compose.yml --profile "*" up -d --build
    }
    else
    {
        Write-host "Preqreqs missing - please install docker or podman"
    }
}

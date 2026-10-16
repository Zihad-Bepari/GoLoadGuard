<?php

namespace GoLoadGuard\Laravel;

use Illuminate\Support\ServiceProvider;

class LoadGuardServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        $router = $this->app['router'] ?? null;
        if ($router) {
            $router->aliasMiddleware('loadguard', LoadGuardMiddleware::class);
        }
    }

    public function register(): void
    {
        // Zero-configuration service registration
    }
}

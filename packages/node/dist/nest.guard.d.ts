import { CanActivate, ExecutionContext, HttpStatus } from '@nestjs/common';
import { LoadGuard } from './index';
/**
 * NestJS Guard for protecting endpoints against overload.
 *
 * Usage:
 * ```typescript
 * @UseGuards(new NestLoadGuard(new LoadGuard({ maxConcurrency: 10, maxQueueSize: 20 })))
 * @Get('/heavy-endpoint')
 * getData() { ... }
 * ```
 */
export declare class NestLoadGuard implements CanActivate {
    private readonly guard;
    private readonly overloadStatus;
    constructor(guard: LoadGuard, overloadStatus?: HttpStatus);
    canActivate(context: ExecutionContext): Promise<boolean>;
}
//# sourceMappingURL=nest.guard.d.ts.map
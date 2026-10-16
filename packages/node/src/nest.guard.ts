import { Injectable, CanActivate, ExecutionContext, HttpException, HttpStatus } from '@nestjs/common';
import { LoadGuard, OverloadedError, ClosedError } from './index';

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
@Injectable()
export class NestLoadGuard implements CanActivate {
  constructor(
    private readonly guard: LoadGuard,
    private readonly overloadStatus: HttpStatus = HttpStatus.SERVICE_UNAVAILABLE
  ) {}

  async canActivate(context: ExecutionContext): Promise<boolean> {
    const http = context.switchToHttp();
    const res = http.getResponse();

    try {
      const release = await this.guard.acquire();
      res.on('finish', release);
      res.on('close', release);
      return true;
    } catch (err) {
      if (err instanceof OverloadedError || err instanceof ClosedError) {
        throw new HttpException('Service Unavailable - System Overloaded', this.overloadStatus);
      }
      throw err;
    }
  }
}

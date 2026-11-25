import { Directive, inject, input, output } from '@angular/core';
import { CardNode } from '../models/card-node.model';
import { RolodexStateService } from '../services/rolodex-state.service';
import { VexBridgeService } from '../services/vex-bridge.service';

@Directive()
export abstract class BaseCardTemplate<T = Record<string, unknown>> {
  readonly node = input.required<CardNode<T>>();
  readonly depth = input<number>(0);
  readonly copyValue = output<{ label: string; value: string }>();

  protected readonly state = inject(RolodexStateService);
  protected readonly bridge = inject(VexBridgeService);

  protected triggerCopy(label: string, value: string): void {
    this.copyValue.emit({ label, value });
    void this.state.copyText(label, value);
    void this.bridge.recordCardUsed(this.node().id);
  }

  protected openExternal(url: string): void {
    this.state.openExternalUrl(url);
    void this.bridge.recordCardUsed(this.node().id);
  }
}
